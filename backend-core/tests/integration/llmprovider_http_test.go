package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	authutil "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/util"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmdto "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestProviderHTTP_RejectsInvalidPathsQueriesAndSecretBodies(t *testing.T) {
	router := httpx.NewRouter(func(context.Context, *errors.AppError) {})
	mountProvidersForTest(router, llmServices{}, 128)
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
		response := send(router, tc.method, tc.path, tc.body, nil)
		require.Equal(t, tc.status, response.Code, tc.path)
		require.NotContains(t, response.Body.String(), "invalid-secret")
	}
}

func TestProviderHTTP_AssignRoleRequiresModelIDField(t *testing.T) {
	router := httpx.NewRouter(nil)
	mountProvidersForTest(router, llmServices{}, 128)
	response := send(router, http.MethodPut, "/api/v1/model-roles/chat.fast", `{}`, nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), `"modelId"`)
	require.Contains(t, response.Body.String(), `"REQUIRED"`)
}

func TestProviderHTTP_RealLoginSessionAndCSRF(t *testing.T) {
	var logs bytes.Buffer
	router, fixture := newProviderRouter(t, &logs)
	authConfig := config.AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "strong-test-password", SessionTTL: time.Hour, MinPasswordLength: 12}
	authService := authservice.NewAuthService(authrepository.NewUserRepo(fixture.pool), authrepository.NewSessionRepo(fixture.pool, 0), fixture.pool, authConfig, time.Now)
	require.NoError(t, authService.EnsureOwner(platformContext(t.Context())))
	limiter := authlimiter.NewLoginLimiter(startRedis(t), config.AuthConfig{LoginMaxAttempts: 10, LoginEmailMaxAttempts: 10, LoginIPMaxAttempts: config.DefaultLoginIPMaxAttempts, LoginWindow: time.Minute}, testLimiterIdentityKey)
	controller := authcontroller.NewAuthController(authcontroller.AuthControllerParams{AuthService: authService, Limiter: limiter})
	router.POST("/api/v1/auth/login", controller.HandleLogin)
	logger, err := logging.NewLogger(&logs, "debug", "llmprovider-test", "test", nil)
	require.NoError(t, err)
	protected := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	mountProvidersForTest(protected.Group("", authmiddleware.RequireSession(authService, false)), fixture.llmServices, testMaxBodyBytes)

	unauthenticated := send(protected, http.MethodGet, "/api/v1/providers", "", nil)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	login := send(router, http.MethodPost, "/api/v1/auth/login", `{"email":"owner@example.com","password":"strong-test-password"}`, nil)
	require.Equal(t, http.StatusOK, login.Code)
	var loginBody struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		}
	}
	require.NoError(t, json.Unmarshal(login.Body.Bytes(), &loginBody))
	var cookieParts []string
	for _, sessionCookie := range login.Result().Cookies() {
		cookieParts = append(cookieParts, sessionCookie.Name+"="+sessionCookie.Value)
	}
	headers := map[string]string{"Cookie": strings.Join(cookieParts, "; ")}
	require.Equal(t, http.StatusOK, send(protected, http.MethodGet, "/api/v1/providers", "", headers).Code)
	createBody := `{"kind":"openrouter","name":"Protected","apiKey":"` + providerSecretKey + `"}`
	require.Equal(t, http.StatusForbidden, send(protected, http.MethodPost, "/api/v1/providers", createBody, headers).Code)
	headers[authutil.CSRFHeader] = loginBody.Data.CSRFToken
	created := send(protected, http.MethodPost, "/api/v1/providers", createBody, headers)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NotContains(t, created.Body.String(), providerSecretKey)
	require.NotContains(t, logs.String(), providerSecretKey)
	require.Contains(t, logs.String(), "request_id")
}

func TestProviderHTTP_CRUDRoutesAndWriteOnlyKeyUpdates(t *testing.T) {
	var logs bytes.Buffer
	router, fixture := newProviderRouter(t, &logs)
	created := send(router, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"Routes","apiKey":"`+providerSecretKey+`"}`, nil)
	require.Equal(t, http.StatusCreated, created.Code)
	var providerBody struct{ Data llmdto.ProviderResponse }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &providerBody))
	providerPath := "/api/v1/providers/" + providerBody.Data.ID.String()
	updated := send(router, http.MethodPatch, providerPath, `{"name":"Routes renamed","apiKey":"replacement-secret"}`, nil)
	require.Equal(t, http.StatusOK, updated.Code)
	require.NotContains(t, updated.Body.String(), "replacement-secret")
	require.Contains(t, updated.Body.String(), `"hasApiKey":true`)
	tested := send(router, http.MethodPost, providerPath+"/test", "", nil)
	require.Equal(t, http.StatusOK, tested.Code)
	require.Contains(t, tested.Body.String(), `"modelCount":0`)
	fixture.catalog.models = []llmmodel.RemoteModel{{ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []llmmodel.Capability{llmmodel.CapabilityChat}}}
	synced := send(router, http.MethodPost, providerPath+"/sync", "", nil)
	require.Equal(t, http.StatusOK, synced.Code)
	require.Contains(t, synced.Body.String(), `"added":1`)
	modelCreated := send(router, http.MethodPost, "/api/v1/models", `{"providerId":"`+providerBody.Data.ID.String()+`","modelRef":"x/manual","displayName":"Manual","capabilities":["chat","tools"]}`, nil)
	require.Equal(t, http.StatusCreated, modelCreated.Code)
	var modelBody struct{ Data llmdto.LLMModelResponse }
	require.NoError(t, json.Unmarshal(modelCreated.Body.Bytes(), &modelBody))
	listed := send(router, http.MethodGet, "/api/v1/models?providerId="+providerBody.Data.ID.String()+"&capability=tools", "", nil)
	require.Equal(t, http.StatusOK, listed.Code)
	var listBody struct{ Data []llmdto.LLMModelResponse }
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &listBody))
	require.Len(t, listBody.Data, 1)
	require.Equal(t, modelBody.Data.ID, listBody.Data[0].ID)
	assigned := send(router, http.MethodPut, "/api/v1/model-roles/chat.default", `{"modelId":"`+modelBody.Data.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, assigned.Code)
	roles := send(router, http.MethodGet, "/api/v1/model-roles", "", nil)
	require.Equal(t, http.StatusOK, roles.Code)
	require.Contains(t, roles.Body.String(), modelBody.Data.ID.String())
	require.Equal(t, http.StatusConflict, send(router, http.MethodDelete, "/api/v1/models/"+modelBody.Data.ID.String(), "", nil).Code)
	require.Equal(t, http.StatusConflict, send(router, http.MethodDelete, providerPath, "", nil).Code)
	removedKey := send(router, http.MethodPatch, providerPath, `{"apiKey":""}`, nil)
	require.Equal(t, http.StatusOK, removedKey.Code)
	require.Contains(t, removedKey.Body.String(), `"hasApiKey":false`)
	syncedModels, err := fixture.models.ListModels(platformContext(t.Context()), &providerBody.Data.ID, nil)
	require.NoError(t, err)
	var removableID uuid.UUID
	for _, llmModel := range syncedModels {
		if llmModel.ID != modelBody.Data.ID {
			removableID = llmModel.ID
		}
	}
	require.NotEqual(t, uuid.Nil, removableID)
	deletedModel := send(router, http.MethodDelete, "/api/v1/models/"+removableID.String(), "", nil)
	require.Equal(t, http.StatusOK, deletedModel.Code)
	require.Contains(t, deletedModel.Body.String(), `"deleted":true`)
	emptyProvider, err := fixture.providers.CreateProvider(platformContext(t.Context()), llmmodel.ProviderInput{Kind: llmmodel.KindOpenRouter, Name: "Empty"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, send(router, http.MethodDelete, "/api/v1/providers/"+emptyProvider.ID.String(), "", nil).Code)
	require.NotContains(t, logs.String(), providerSecretKey)
	require.NotContains(t, logs.String(), "replacement-secret")
	var metadata string
	require.NoError(t, fixture.pool.QueryRow(platformContext(t.Context()), "SELECT string_agg(metadata::text, ' ') FROM audit_events").Scan(&metadata))
	require.NotContains(t, metadata, providerSecretKey)
	require.NotContains(t, metadata, "replacement-secret")
}

func TestProviderHTTP_KeyNeverLeaks(t *testing.T) {
	var logs bytes.Buffer
	router, fixture := newProviderRouter(t, &logs)
	created := send(router, http.MethodPost, "/api/v1/providers", `{"kind":"openrouter","name":"OR","apiKey":"`+providerSecretKey+`"}`, nil)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NotContains(t, created.Body.String(), providerSecretKey)
	require.Contains(t, created.Body.String(), `"hasApiKey":true`)
	invalid := send(router, http.MethodPost, "/api/v1/providers", `{"kind":"bogus","name":"X","apiKey":"`+providerSecretKey+`"}`, nil)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.NotContains(t, invalid.Body.String(), providerSecretKey)
	require.NotContains(t, send(router, http.MethodGet, "/api/v1/providers", "", nil).Body.String(), providerSecretKey)
	var auditMetadata string
	require.NoError(t, fixture.pool.QueryRow(platformContext(context.Background()), "SELECT string_agg(metadata::text, ' ') FROM audit_events").Scan(&auditMetadata))
	require.NotContains(t, auditMetadata, providerSecretKey)
	require.NotContains(t, logs.String(), providerSecretKey)
}

func TestProviderHTTP_RoleAssignmentChecksCapabilities(t *testing.T) {
	var logs bytes.Buffer
	router, fixture := newProviderRouter(t, &logs)
	provider := seedProvider(t, fixture)
	llmModel, err := fixture.models.CreateModel(platformContext(context.Background()), llmmodel.LLMModelInput{ProviderID: provider.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []llmmodel.Capability{llmmodel.CapabilityChat}})
	require.NoError(t, err)
	missingTools := send(router, http.MethodPut, "/api/v1/model-roles/chat.default", `{"modelId":"`+llmModel.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, missingTools.Code)
	assigned := send(router, http.MethodPut, "/api/v1/model-roles/chat.fast", `{"modelId":"`+llmModel.ID.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, assigned.Code)
	require.NotContains(t, assigned.Body.String(), providerSecretKey)
}
