package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	appcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/controller"
	appmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/model"
	appservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/service"
	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	orgcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/controller"
	orgmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/middleware"
	orgmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	orgrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	orgservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	proxyBase       = "/api/v1/orgs/"
	proxyTestRoute  = "/api/v1/orgs/:orgId/apps/ragx/*path"
	proxyLoggedPath = "/api/v1/orgs/{orgId}/apps/ragx/{path}"
)

func proxyPath(orgID, rest string) string { return proxyBase + orgID + "/apps/ragx/" + rest }

// mountAppProxyForTest registers the proxy exactly as cmd/core/routes.go does.
func mountAppProxyForTest(routes gin.IRoutes, controller *appcontroller.AppProxyController) {
	for _, method := range appmodel.ForwardedMethods() {
		routes.Handle(method, proxyTestRoute, controller.HandleForward)
	}
}

// proxyServices wires the real organization and proxy layers on one caller of the fake ragx.
func proxyServices(t *testing.T, fake *fakeRagx, pool *pgxpool.Pool, logs *bytes.Buffer) (orgservice.OrganizationService, *orgcontroller.OrganizationController, *appcontroller.AppProxyController) {
	t.Helper()
	caller := fake.caller(t, logs)
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	organizations := orgservice.NewOrganizationService(pool, orgrepository.NewOrganizationRepo(), caller, policy, uuid.NewString)
	return organizations, orgcontroller.NewOrganizationController(organizations, testMaxBodyBytes),
		appcontroller.NewAppProxyController(appservice.NewAppProxyService(caller), testMaxBodyBytes)
}

func newAppProxyRouter(t *testing.T, fake *fakeRagx, routeTimeout time.Duration) (*gin.Engine, string, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	log := logger(t, logs)
	organizations, orgController, proxyController := proxyServices(t, fake, pgtest.Start(t), logs)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(log), httpx.LogAccess(log))
	router.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(platformContext(ctx.Request.Context()))
		ctx.Next()
	})
	router.Use(httpx.LimitRequestDuration(routeTimeout))
	mountOrganizationsForTest(router, orgController)
	mountAppProxyForTest(router.Group("", orgmiddleware.RequireOrg(organizations)), proxyController)
	status, org := createOrg(t, router, "Proxy org")
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "ready", org["ragxStatus"])
	return router, org["id"].(string), logs
}

func TestAppProxy_SignsOidFromTheCheckedOrgAndRidFromTheRequest(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	fake.setResponder(ragxJSON(http.StatusOK, `{"data":[],"meta":{"requestId":"x"}}`))
	res := send(router, http.MethodGet, proxyPath(orgID, "knowledge-bases?limit=5"), "", map[string]string{"X-Request-Id": "req_proxy-read-0001"})
	require.Equal(t, http.StatusOK, res.Code)
	require.JSONEq(t, `{"data":[],"meta":{"requestId":"x"}}`, res.Body.String(), "2xx bodies pass through verbatim")
	require.Equal(t, "no-store", res.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", res.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "application/json", res.Header().Get("Content-Type"))
	last := fake.calls()[len(fake.calls())-1]
	require.Equal(t, "/v1/knowledge-bases", last.path)
	require.Equal(t, "limit=5", last.rawQuery)
	require.Equal(t, orgID, last.claims["oid"])
	require.Equal(t, "kb:read", last.claims["scp"])
	require.Equal(t, "req_proxy-read-0001", last.claims["rid"])
	require.Equal(t, "req_proxy-read-0001", last.header.Get("X-Request-Id"))
	require.NotContains(t, last.claims, "kb")
}

func TestAppProxy_ScopeFollowsTheMethod(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	fake.setResponder(ragxStatus(http.StatusNoContent))
	for method, scope := range map[string]string{
		http.MethodGet: "kb:read", http.MethodHead: "kb:read", http.MethodPost: "kb:write",
		http.MethodPut: "kb:write", http.MethodPatch: "kb:write", http.MethodDelete: "kb:write",
	} {
		res := send(router, method, proxyPath(orgID, "knowledge-bases/k"), "", nil)
		require.Equal(t, http.StatusNoContent, res.Code, method)
		require.Empty(t, res.Body.Bytes(), "204 writes no body")
		calls := fake.calls()
		require.Equal(t, method, calls[len(calls)-1].method)
		require.Equal(t, scope, calls[len(calls)-1].claims["scp"], method)
	}
}

func TestAppProxy_RejectsUnsafePathsWithoutCallingRagx(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	before := len(fake.calls())
	for _, rest := range []string{"", ".", "knowledge-bases/.", "./knowledge-bases", "knowledge-bases/", "a//b", "../orgs/x", "knowledge-bases/..", "%2e%2e/x", "kb%20x", "kb;x"} {
		res := send(router, http.MethodGet, proxyPath(orgID, rest), "", nil)
		require.Equal(t, http.StatusNotFound, res.Code, rest)
		require.Contains(t, res.Body.String(), `"code":"NOT_FOUND"`, rest)
	}
	require.Len(t, fake.calls(), before, "no unsafe path reaches ragx")
}

func TestAppProxy_ProvisionRouteIsNotForTheUI(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	// Like ragx: PUT /v1/orgs/{id} needs org:provision; the UI token is kb:write, so ragx answers 403.
	fake.setResponder(func(w http.ResponseWriter, r *http.Request) { ragxError(http.StatusForbidden, "FORBIDDEN")(w, r) })
	res := send(router, http.MethodPut, proxyPath(orgID, "orgs/"+orgID), `{"name":"x","externalParsing":true}`, nil)
	require.Equal(t, http.StatusNotFound, res.Code)
	require.Contains(t, res.Body.String(), `"code":"NOT_FOUND"`)
	calls := fake.calls()
	require.Equal(t, "kb:write", calls[len(calls)-1].claims["scp"], "core never signs org:provision for the UI")
}

func TestAppProxy_ForwardsOnlyAllowListedHeaders(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	fake.setResponder(ragxJSON(http.StatusCreated, `{"data":{},"meta":{"requestId":"x"}}`))
	body := `{"name":"Chính sách"}`
	res := send(router, http.MethodPost, proxyPath(orgID, "knowledge-bases"), body, map[string]string{
		"Cookie": "thomas_session=secret-cookie", "Authorization": "Bearer client-supplied", "X-Forwarded-For": "10.0.0.1",
		"X-Custom": "1", "Idempotency-Key": "kb-create-0001", "Accept-Language": "en",
	})
	require.Equal(t, http.StatusCreated, res.Code)
	calls := fake.calls()
	got := calls[len(calls)-1]
	require.NotNil(t, got.claims, "the fake verified core's own token")
	require.NotEqual(t, "Bearer client-supplied", got.header.Get("Authorization"))
	for _, absent := range []string{"Cookie", "X-Forwarded-For", "X-Custom"} {
		require.Empty(t, got.header.Get(absent), absent)
	}
	require.Equal(t, "kb-create-0001", got.header.Get("Idempotency-Key"))
	require.Equal(t, "en", got.header.Get("Accept-Language"))
	require.Equal(t, "application/json", got.header.Get("Content-Type"))
	require.Equal(t, body, string(got.body))
}

func TestAppProxy_RejectsBadRequestsBeforeCallingRagx(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	before := len(fake.calls())
	tooLarge := send(router, http.MethodPost, proxyPath(orgID, "knowledge-bases"), strings.Repeat("x", testMaxBodyBytes+1), nil)
	require.Equal(t, http.StatusRequestEntityTooLarge, tooLarge.Code)
	badKey := send(router, http.MethodPost, proxyPath(orgID, "knowledge-bases"), `{}`, map[string]string{"Idempotency-Key": "short"})
	require.Equal(t, http.StatusBadRequest, badKey.Code)
	require.Contains(t, badKey.Body.String(), `"field":"Idempotency-Key"`)
	require.Len(t, fake.calls(), before)
}

func TestAppProxy_MapsRagxResponsesPerTheErrorTable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		respond http.HandlerFunc
		status  int
		code    string // the body's error code
	}{
		{"catalog 404 passes", ragxError(http.StatusNotFound, "KB_NOT_FOUND"), http.StatusNotFound, "KB_NOT_FOUND"},
		{"unmatched ragx route passes as NOT_FOUND", ragxError(http.StatusNotFound, "NOT_FOUND"), http.StatusNotFound, "NOT_FOUND"},
		{"framework 404 without the shared format", ragxJSON(http.StatusNotFound, `{"detail":"Not Found"}`), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"catalog 409 passes", ragxError(http.StatusConflict, "CONFLICT"), http.StatusConflict, "CONFLICT"},
		{"catalog 400 passes", ragxError(http.StatusBadRequest, "VALIDATION_FAILED"), http.StatusBadRequest, "VALIDATION_FAILED"},
		{"401 is a core misconfiguration", ragxError(http.StatusUnauthorized, "APP_TOKEN_INVALID"), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"403 hides the route", ragxError(http.StatusForbidden, "FORBIDDEN"), http.StatusNotFound, "NOT_FOUND"},
		{"5xx", ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"unknown code", ragxError(http.StatusNotFound, "NOT_A_CODE"), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"code and status drift", ragxError(http.StatusBadRequest, "KB_NOT_FOUND"), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
		}, http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"body over the cap", ragxJSON(http.StatusOK, `{"data":"`+strings.Repeat("x", 70<<10)+`"}`), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
		{"slower than the route", slowly(500*time.Millisecond, ragxJSON(http.StatusOK, `{"data":[]}`)), http.StatusServiceUnavailable, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeRagx(t)
			router, orgID, logs := newAppProxyRouter(t, fake, 200*time.Millisecond)
			fake.setResponder(tc.respond)
			res := send(router, http.MethodGet, proxyPath(orgID, "knowledge-bases/k"), "", nil)
			require.Equal(t, tc.status, res.Code)
			require.Contains(t, res.Body.String(), `"code":"`+tc.code+`"`)
			require.Contains(t, res.Body.String(), `"requestId":"`)
			if tc.status == http.StatusServiceUnavailable {
				// 5xx bodies always say INTERNAL_ERROR (rule 4); the real code is in the boundary log only.
				require.Contains(t, logs.String(), `"code":"RAGX_UNAVAILABLE"`)
			}
		})
	}
}

func TestAppProxy_UnreachableRagxIsUnavailable(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, _ := newAppProxyRouter(t, fake, time.Minute)
	fake.server.Close() // the org is ready; ragx is now gone
	res := send(router, http.MethodGet, proxyPath(orgID, "knowledge-bases"), "", nil)
	require.Equal(t, http.StatusServiceUnavailable, res.Code)
}

func TestAppProxy_RefusesOrgsThatAreNotReadyWithoutCallingRagx(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, _ := newAppProxyRouter(t, fake, time.Minute)
	fake.setResponder(ragxError(http.StatusNotFound, "LLM_ROLE_NOT_ASSIGNED"))
	_, failed := createOrg(t, router, "Not ready")
	before := len(fake.calls())
	for id, want := range map[string]int{
		failed["id"].(string): http.StatusConflict, tenant.PlatformID: http.StatusNotFound,
		"not-a-uuid": http.StatusNotFound, uuid.NewString(): http.StatusNotFound,
	} {
		res := send(router, http.MethodGet, proxyPath(id, "knowledge-bases"), "", nil)
		require.Equal(t, want, res.Code, id)
	}
	require.Len(t, fake.calls(), before)
}

func TestAppProxy_LogsOneRagxCallLineWithoutTokenOrBody(t *testing.T) {
	fake := newFakeRagx(t)
	router, orgID, logs := newAppProxyRouter(t, fake, time.Minute)
	fake.setResponder(ragxJSON(http.StatusOK, `{"data":"secret-body-marker"}`))
	logs.Reset()
	send(router, http.MethodGet, proxyPath(orgID, "knowledge-bases"), "", map[string]string{"X-Request-Id": "req_proxy-log-0001"})
	var calls []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), line)
		if record["msg"] == "ragx call" {
			calls = append(calls, record)
		}
	}
	require.Len(t, calls, 1)
	record := calls[0]
	require.Equal(t, "GET", record["method"])
	require.Equal(t, proxyLoggedPath, record["route"], "the same {orgId} spelling as the access log")
	require.EqualValues(t, http.StatusOK, record["status"])
	require.Contains(t, record, "duration_ms")
	require.Equal(t, "req_proxy-log-0001", record["request_id"])
	require.NotEmpty(t, record["jti"])
	require.NotContains(t, logs.String(), "secret-body-marker")
	require.NotContains(t, logs.String(), "Bearer")
}

func TestAppProxy_RequiresSessionAndCSRF(t *testing.T) {
	fake := newFakeRagx(t)
	logs := &bytes.Buffer{}
	log := logger(t, logs)
	pool := pgtest.Start(t)
	organizations, _, proxyController := proxyServices(t, fake, pool, logs)
	externalParsing := true
	org, err := organizations.CreateOrganization(platformContext(t.Context()), orgmodel.OrganizationInput{Name: "Session org", ExternalParsing: &externalParsing})
	require.NoError(t, err)
	authConfig := config.AuthConfig{OwnerEmail: "owner@example.com", OwnerPassword: "strong-test-password", SessionTTL: time.Hour, MinPasswordLength: 12}
	authService := authservice.NewAuthService(authrepository.NewUserRepo(pool), authrepository.NewSessionRepo(pool, 0), pool, authConfig, time.Now)
	require.NoError(t, authService.EnsureOwner(platformContext(t.Context())))
	limiter := authlimiter.NewLoginLimiter(startRedis(t), config.AuthConfig{LoginMaxAttempts: 10, LoginEmailMaxAttempts: 10, LoginIPMaxAttempts: config.DefaultLoginIPMaxAttempts, LoginWindow: time.Minute}, testLimiterIdentityKey)
	loginRouter := httpx.NewRouter(httpx.NewSlogErrorLogger(log), httpx.LogAccess(log))
	loginRouter.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(platformContext(ctx.Request.Context()))
		ctx.Next()
	})
	loginRouter.POST("/api/v1/auth/login", authcontroller.NewAuthController(authcontroller.AuthControllerParams{AuthService: authService, Limiter: limiter}).HandleLogin)
	protected := httpx.NewRouter(httpx.NewSlogErrorLogger(log), httpx.LogAccess(log))
	session := protected.Group("", authmiddleware.RequireSession(authService, false))
	mountAppProxyForTest(session.Group("", orgmiddleware.RequireOrg(organizations)), proxyController)

	path := proxyPath(org.ID, "knowledge-bases")
	unauthenticated := send(protected, http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	require.Contains(t, unauthenticated.Body.String(), `"code":"UNAUTHENTICATED"`)

	login := send(loginRouter, http.MethodPost, "/api/v1/auth/login", `{"email":"owner@example.com","password":"strong-test-password"}`, nil)
	require.Equal(t, http.StatusOK, login.Code)
	var cookieParts []string
	for _, sessionCookie := range login.Result().Cookies() {
		cookieParts = append(cookieParts, sessionCookie.Name+"="+sessionCookie.Value)
	}
	headers := map[string]string{"Cookie": strings.Join(cookieParts, "; ")}
	provisionCalls := len(fake.calls())

	noCSRF := send(protected, http.MethodPost, path, `{}`, headers)
	require.Equal(t, http.StatusForbidden, noCSRF.Code)
	require.Contains(t, noCSRF.Body.String(), `"code":"AUTH_CSRF_INVALID"`)
	fake.setResponder(ragxJSON(http.StatusOK, `{"data":[]}`))
	require.Equal(t, http.StatusOK, send(protected, http.MethodGet, path, "", headers).Code)
	require.Len(t, fake.calls(), provisionCalls+1, "only the GET reached ragx")
}
