package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	serviceToken     = "svc-token"
	testMaxBodyBytes = 64 << 10
)

func newRouter(t *testing.T, logs *bytes.Buffer) (chi.Router, *Service) {
	t.Helper()
	s := newTestService(t, &fakeCatalog{})
	log, err := logging.NewLogger(logs, "debug", "test", "test", nil)
	require.NoError(t, err)
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(log), registryTestTracing(t), httpx.TraceRequests("registry-test"), httpx.LogAccess(log))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(platformContext(r.Context())))
		})
	})
	NewHandler(s, testMaxBodyBytes).Mount(r)
	MountInternal(r, s, serviceToken)
	return r, s
}

func TestServiceTokenSetsExplicitPlatformTenant(t *testing.T) {
	handler := requireServiceToken(serviceToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := tenant.ID(r.Context())
		require.NoError(t, err)
		require.Equal(t, "default", got)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/internal/probe", nil)
	request.Header.Set("Authorization", "Bearer "+serviceToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request.WithContext(tenant.WithID(request.Context(), "untrusted")))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func registryTestTracing(t *testing.T) httpx.Middleware {
	t.Helper()
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(platformContext(context.Background()))) })
	tracer := provider.Tracer("registry-test")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			ctx, span := tracer.Start(request.Context(), "registry-test.request")
			defer span.End()
			next.ServeHTTP(w, request.WithContext(ctx))
		})
	}
}

func TestServiceToken_RejectsMissingBearerPrefixAndEmptyConfiguration(t *testing.T) {
	for _, token := range []string{serviceToken, ""} {
		for _, header := range []string{"", serviceToken, "Basic " + serviceToken, "Bearer ", "Bearer wrong"} {
			called := false
			router := httpx.NewRouter(func(context.Context, *errors.AppError) {})
			router.With(requireServiceToken(token)).Get("/probe", func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) })
			res := send(router, http.MethodGet, "/probe", "", map[string]string{"Authorization": header})
			require.Equal(t, http.StatusUnauthorized, res.Code, token+"/"+header)
			require.False(t, called)
			require.NotContains(t, res.Body.String(), serviceToken)
		}
	}
}

func TestServiceTokenAcceptsMixedCaseBearerAndNeverCaches(t *testing.T) {
	router := httpx.NewRouter(nil)
	router.With(requireServiceToken(serviceToken)).Get("/probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, fixture := range []struct {
		header string
		status int
	}{{"bEaReR " + serviceToken, 204}, {"Bearer wrong", 401}} {
		response := send(router, http.MethodGet, "/probe", "", map[string]string{"Authorization": fixture.header})
		require.Equal(t, fixture.status, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
}

func TestRegistryHTTP_RejectsInvalidPathsQueriesAndSecretBodies(t *testing.T) {
	router := httpx.NewRouter(func(context.Context, *errors.AppError) {})
	NewHandler(nil, 128).Mount(router)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodDelete, "/api/v1/providers/invalid-secret", "", http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/models/invalid-secret", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/models?providerId=invalid-secret", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/models?capability=invalid-secret", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/providers", `{"apiKey":123,"secret":"invalid-secret"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/providers", `{"apiKey":"invalid-secret` + strings.Repeat("x", 128) + `"}`, http.StatusRequestEntityTooLarge},
		{http.MethodPut, "/api/v1/model-roles/chat.fast", `{"modelId":"invalid-secret"}`, http.StatusBadRequest},
	} {
		rec := send(router, tc.method, tc.path, tc.body, nil)
		require.Equal(t, tc.status, rec.Code, tc.path)
		require.NotContains(t, rec.Body.String(), "invalid-secret")
	}
}

func TestAssignRoleRequiresModelIDField(t *testing.T) {
	router := httpx.NewRouter(nil)
	NewHandler(nil, 128).Mount(router)
	response := send(router, http.MethodPut, "/api/v1/model-roles/chat.fast", `{}`, nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), `"modelId"`)
	require.Contains(t, response.Body.String(), `"REQUIRED"`)
}

func registryRedis(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(platformContext(t.Context()), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(platformContext(context.Background()))) })
	redisURL, err := container.ConnectionString(platformContext(t.Context()))
	require.NoError(t, err)
	opts, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestRegistryHTTP_RealLoginSessionAndCSRF(t *testing.T) {
	var logs bytes.Buffer
	router, svc := newRouter(t, &logs)
	authCfg := config.AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "strong-test-password", SessionTTL: time.Hour, MinPasswordLength: 12}
	authSvc := auth.NewService(auth.NewRepository(svc.pool), svc.pool, authCfg, time.Now)
	require.NoError(t, authSvc.EnsureOwner(platformContext(t.Context())))
	auth.NewHandler(authSvc, auth.NewLimiter(registryRedis(t), 10, time.Minute), false).Mount(router)
	logger, err := logging.NewLogger(&logs, "debug", "registry-test", "test", nil)
	require.NoError(t, err)
	protected := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), registryTestTracing(t), httpx.TraceRequests("registry-test"), httpx.LogAccess(logger))
	protected.Use(auth.RequireSession(authSvc))
	NewHandler(svc, testMaxBodyBytes).Mount(protected)
	unauth := send(protected, http.MethodGet, "/api/v1/providers", "", nil)
	require.Equal(t, http.StatusUnauthorized, unauth.Code)
	login := send(router, http.MethodPost, "/api/v1/auth/login", `{"email":"owner@example.com","password":"strong-test-password"}`, nil)
	require.Equal(t, http.StatusOK, login.Code)
	var response struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		}
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &response))
	var cookieParts []string
	for _, cookie := range login.Result().Cookies() {
		cookieParts = append(cookieParts, cookie.Name+"="+cookie.Value)
	}
	headers := map[string]string{"Cookie": strings.Join(cookieParts, "; ")}
	list := send(protected, http.MethodGet, "/api/v1/providers", "", headers)
	require.Equal(t, http.StatusOK, list.Code)
	denied := send(protected, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"Protected","apiKey":"`+secretKey+`"}`, headers)
	require.Equal(t, http.StatusForbidden, denied.Code)
	headers[auth.HeaderCSRF] = response.Data.CSRFToken
	created := send(protected, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"Protected","apiKey":"`+secretKey+`"}`, headers)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NotContains(t, created.Body.String(), secretKey)
	require.NotContains(t, logs.String(), secretKey)
	require.Contains(t, logs.String(), "trace_id")
}

func TestRegistryHTTP_CRUDRoutesAndWriteOnlyKeyUpdates(t *testing.T) {
	var logs bytes.Buffer
	router, svc := newRouter(t, &logs)
	created := send(router, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"Routes","apiKey":"`+secretKey+`"}`, nil)
	require.Equal(t, http.StatusCreated, created.Code)
	var providerResponse struct{ Data Provider }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &providerResponse))
	providerPath := "/api/v1/providers/" + providerResponse.Data.ID.String()
	updated := send(router, http.MethodPatch, providerPath, `{"name":"Routes renamed","apiKey":"replacement-secret"}`, nil)
	require.Equal(t, http.StatusOK, updated.Code)
	require.NotContains(t, updated.Body.String(), "replacement-secret")
	require.Contains(t, updated.Body.String(), `"hasApiKey":true`)
	tested := send(router, http.MethodPost, providerPath+"/test", "", nil)
	require.Equal(t, http.StatusOK, tested.Code)
	require.Contains(t, tested.Body.String(), `"modelCount":0`)
	svc.catalog.(*fakeCatalog).models = []RemoteModel{{ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapChat}}}
	synced := send(router, http.MethodPost, providerPath+"/sync", "", nil)
	require.Equal(t, http.StatusOK, synced.Code)
	require.Contains(t, synced.Body.String(), `"added":1`)
	modelCreated := send(router, http.MethodPost, "/api/v1/models", `{"providerId":"`+providerResponse.Data.ID.String()+`","modelRef":"x/manual","displayName":"Manual","capabilities":["chat","tools"]}`, nil)
	require.Equal(t, http.StatusCreated, modelCreated.Code)
	var modelResponse struct{ Data Model }
	require.NoError(t, json.Unmarshal(modelCreated.Body.Bytes(), &modelResponse))
	models := send(router, http.MethodGet, "/api/v1/models?providerId="+providerResponse.Data.ID.String()+"&capability=tools", "", nil)
	require.Equal(t, http.StatusOK, models.Code)
	var listResponse struct{ Data []Model }
	require.NoError(t, json.Unmarshal(models.Body.Bytes(), &listResponse))
	require.Len(t, listResponse.Data, 1)
	require.Equal(t, modelResponse.Data.ID, listResponse.Data[0].ID)
	assigned := send(router, http.MethodPut, "/api/v1/model-roles/chat.default", `{"modelId":"`+modelResponse.Data.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, assigned.Code)
	roles := send(router, http.MethodGet, "/api/v1/model-roles", "", nil)
	require.Equal(t, http.StatusOK, roles.Code)
	require.Contains(t, roles.Body.String(), modelResponse.Data.ID.String())
	inUse := send(router, http.MethodDelete, "/api/v1/models/"+modelResponse.Data.ID.String(), "", nil)
	require.Equal(t, http.StatusConflict, inUse.Code)
	providerInUse := send(router, http.MethodDelete, providerPath, "", nil)
	require.Equal(t, http.StatusConflict, providerInUse.Code)
	removedKey := send(router, http.MethodPatch, providerPath, `{"apiKey":""}`, nil)
	require.Equal(t, http.StatusOK, removedKey.Code)
	require.Contains(t, removedKey.Body.String(), `"hasApiKey":false`)
	syncModels, err := svc.ListModels(platformContext(t.Context()), &providerResponse.Data.ID, nil)
	require.NoError(t, err)
	var removableID uuid.UUID
	for _, model := range syncModels {
		if model.ID != modelResponse.Data.ID {
			removableID = model.ID
		}
	}
	require.NotEqual(t, uuid.Nil, removableID)
	deletedModel := send(router, http.MethodDelete, "/api/v1/models/"+removableID.String(), "", nil)
	require.Equal(t, http.StatusOK, deletedModel.Code)
	require.Contains(t, deletedModel.Body.String(), `"deleted":true`)
	emptyProvider, err := svc.CreateProvider(platformContext(t.Context()), ProviderInput{Kind: KindOpenRouter, Name: "Empty"})
	require.NoError(t, err)
	deletedProvider := send(router, http.MethodDelete, "/api/v1/providers/"+emptyProvider.ID.String(), "", nil)
	require.Equal(t, http.StatusOK, deletedProvider.Code)
	require.NotContains(t, logs.String(), secretKey)
	require.NotContains(t, logs.String(), "replacement-secret")
	var metadata string
	require.NoError(t, svc.pool.QueryRow(platformContext(t.Context()), "SELECT string_agg(metadata::text, ' ') FROM audit_events").Scan(&metadata))
	require.NotContains(t, metadata, secretKey)
	require.NotContains(t, metadata, "replacement-secret")
}

func send(r http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestProviderKeyNeverLeaks(t *testing.T) {
	var logs bytes.Buffer
	r, s := newRouter(t, &logs)
	created := send(r, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"OR","apiKey":"`+secretKey+`"}`, nil)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NotContains(t, created.Body.String(), secretKey)
	require.Contains(t, created.Body.String(), `"hasApiKey":true`)

	invalid := send(r, http.MethodPost, "/api/v1/providers", `{"kind":"bogus","name":"X","apiKey":"`+secretKey+`"}`, nil)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.NotContains(t, invalid.Body.String(), secretKey)

	list := send(r, http.MethodGet, "/api/v1/providers", "", nil)
	require.NotContains(t, list.Body.String(), secretKey)

	var auditMeta string
	require.NoError(t, s.pool.QueryRow(platformContext(context.Background()), "SELECT string_agg(metadata::text, ' ') FROM audit_events").Scan(&auditMeta))
	require.NotContains(t, auditMeta, secretKey)
	require.NotContains(t, logs.String(), secretKey)
}

func TestInternalResolveRequiresServiceToken(t *testing.T) {
	var logs bytes.Buffer
	r, _ := newRouter(t, &logs)
	rec := send(r, http.MethodGet, "/internal/models/resolve?role=chat.fast", "", map[string]string{"Authorization": "Bearer wrong"})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var env struct{ Error struct{ Code string } }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, "INTERNAL_TOKEN_INVALID", env.Error.Code)
}

func TestRoleAssignmentOverHTTP(t *testing.T) {
	var logs bytes.Buffer
	r, s := newRouter(t, &logs)
	p := seedProvider(t, s)
	m, err := s.CreateModel(platformContext(context.Background()), ModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapChat}})
	require.NoError(t, err)

	bad := send(r, http.MethodPut, "/api/v1/model-roles/chat.default", `{"modelId":"`+m.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, bad.Code)

	ok := send(r, http.MethodPut, "/api/v1/model-roles/chat.fast", `{"modelId":"`+m.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, ok.Code)

	resolved := send(r, http.MethodGet, "/internal/models/resolve?role=chat.fast", "", map[string]string{"Authorization": "bEaReR " + serviceToken})
	require.Equal(t, "no-store", resolved.Header().Get("Cache-Control"))
	require.Equal(t, http.StatusOK, resolved.Code)
	require.Contains(t, resolved.Body.String(), secretKey, "only the internal endpoint returns the key")
}
