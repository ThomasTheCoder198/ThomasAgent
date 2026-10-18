package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/vault"
)

func TestAPI_GeneratedPostmanCollection(t *testing.T) {
	pool := pgtest.Start(t)
	redisContainer, err := tcredis.Run(t.Context(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, redisContainer.Terminate(context.Background())) })
	redisURL, err := redisContainer.ConnectionString(t.Context())
	require.NoError(t, err)
	setIdentityEnv(t)
	t.Setenv("CORE_DATABASE_URL", pool.Config().ConnConfig.ConnString())
	t.Setenv("CORE_REDIS_URL", redisURL)
	t.Setenv("CORE_AUTH_OWNER_EMAIL", "api-test@example.com")
	t.Setenv("CORE_AUTH_OWNER_PASSWORD", "fixture-password-collection")
	t.Setenv("CORE_AUTH_LOGIN_LIMITER_KEY", "fixture-api-login-limiter-key-with-at-least-32-characters")
	t.Setenv("CORE_RAGX_URL", newCollectionRagx(t).URL)
	cfg, err := config.Load()
	require.NoError(t, err)
	app := &coreRuntime{
		cfg:     cfg,
		log:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
		metrics: metrics.New(cfg.Metrics.HTTPBuckets, cfg.Metrics.LLMBuckets),
	}
	redisClient, err := app.openRedis(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	cipher, err := vault.NewCipher(cfg.Vault.KeyID, cfg.Vault.MasterKey)
	require.NoError(t, err)
	signingKey, err := apptoken.LoadSigningKey(cfg.App, os.ReadFile)
	require.NoError(t, err)
	clients, err := app.initClientContainer(signingKey)
	require.NoError(t, err)
	services, err := app.initServiceContainer(t.Context(), pool, cipher, clients, app.initRepositoryContainer(pool))
	require.NoError(t, err)
	controllers, err := app.initControllerContainer(services, redisClient)
	require.NoError(t, err)
	gin.SetMode(gin.ReleaseMode)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(app.log))
	registerRoutes(router, controllers, middlewares{authService: services.AuthService, cookieSecure: cfg.Auth.CookieSecure, health: app.readinessChecks(pool, redisClient), requestTimeout: cfg.HTTP.RequestTimeout, organizations: services.OrganizationService})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	runCollection(t, server.URL)
}

const (
	collectionOrgPrefix = "/v1/orgs/"
	collectionKBPrefix  = "/v1/knowledge-bases/"
	collectionKBID      = "8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f"
)

// newCollectionRagx answers the two ragx calls the collection needs: provisioning an org (so it ends ready and its
// org-scoped requests, the Agents folder among them, pass RequireOrg) and reading the default KB. It does not verify
// App tokens; the integration suite's fake ragx does.
func newCollectionRagx(t *testing.T) *httptest.Server {
	t.Helper()
	reply := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"requestId": r.Header.Get("X-Request-Id")}})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT "+collectionOrgPrefix+"{orgId}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name            string `json:"name"`
			ExternalParsing bool   `json:"externalParsing"`
			SettingsVersion int64  `json:"settingsVersion"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		reply(w, r, map[string]any{"orgId": r.PathValue("orgId"), "name": body.Name, "externalParsing": body.ExternalParsing,
			"settingsVersion": body.SettingsVersion, "defaultKbId": collectionKBID, "createdAt": time.Now().UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("GET "+collectionKBPrefix+"{kbId}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("kbId") != collectionKBID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "KB_NOT_FOUND"},
				"meta": map[string]any{"requestId": r.Header.Get("X-Request-Id")}})
			return
		}
		reply(w, r, map[string]any{"id": r.PathValue("kbId"), "name": "Mặc định", "isDefault": true, "embeddingModel": "m", "embeddingDims": 1536,
			"fileCount": 0, "createdAt": "2026-10-10T00:00:00Z", "updatedAt": "2026-10-10T00:00:00Z"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestCollectionRagxRejectsUnknownKB(t *testing.T) {
	server := newCollectionRagx(t)
	response, err := http.Get(server.URL + collectionKBPrefix + uuid.NewString())
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	require.Equal(t, http.StatusNotFound, response.StatusCode)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "KB_NOT_FOUND", body.Error.Code)
}

func runCollection(t *testing.T, baseURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	nodePath, err := exec.LookPath("node")
	require.NoError(t, err)
	command := exec.CommandContext(ctx, nodePath, "postman/run.mjs")
	command.Dir = "../../../contracts"
	command.Env = append(os.Environ(), "CORE_API_TEST_BASE_URL="+baseURL)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	require.NoError(t, err, "generated collection must pass against an isolated real API")
}
