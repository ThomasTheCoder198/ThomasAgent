package main

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/vault"
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
	t.Setenv("CORE_SERVICE_TOKEN", "fixture-api-service-token")
	cfg, err := config.Load()
	require.NoError(t, err)
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	app := &application{cfg: cfg, log: slog.New(slog.NewJSONHandler(io.Discard, nil)), startupTracer: provider.Tracer("api-test")}
	redisClient, err := app.openRedis(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	cipher, err := vault.NewCipher(cfg.Vault.KeyID, cfg.Vault.MasterKey)
	require.NoError(t, err)
	router, err := app.buildRouter(t.Context(), pool, redisClient, cipher)
	require.NoError(t, err)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	runCollection(t, server.URL)
}

func runCollection(t *testing.T, baseURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "task", "api-test")
	command.Dir = "../../.."
	command.Env = append(os.Environ(), "CORE_API_TEST_BASE_URL="+baseURL)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	require.NoError(t, err, "generated collection must pass against an isolated real API")
}
