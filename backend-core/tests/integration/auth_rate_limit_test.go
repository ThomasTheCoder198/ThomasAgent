package integration

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func attemptFrom(h http.Handler, email, peer, forwarded string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"wrong-password"}`))
	req = req.WithContext(tenant.WithID(req.Context(), tenant.PlatformID))
	req.RemoteAddr = peer
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", forwarded)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLogin_IgnoresSpoofedForwardingFromUntrustedPeer(t *testing.T) {
	svc, _ := newAuthFixture(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger))
	mountAuthForTest(router, authcontroller.AuthControllerParams{AuthService: svc, Limiter: newTestLimiter(startRedis(t), 1), TrustedProxyHeader: "X-Forwarded-For", Logger: logger})
	require.Equal(t, 401, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.1").Code)
	require.Equal(t, 429, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.2").Code)
}

func TestLogin_EmailBudgetBlocksRotatingIPs(t *testing.T) {
	h, _ := newAuthServer(t, 2)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.1:9999", "").Code)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.2:9999", "").Code)
	require.Equal(t, 429, attemptFrom(h, ownerEmail, "192.0.2.3:9999", "").Code)
}

func TestLogin_IPBudgetBlocksEmailSpray(t *testing.T) {
	t.Setenv("CORE_DATABASE_URL", "postgres://unused")
	t.Setenv("CORE_REDIS_URL", "redis://unused")
	t.Setenv("CORE_VAULT_MASTER_KEY", "unused")
	t.Setenv("CORE_AUTH_LOGIN_LIMITER_KEY", "fedcba9876543210fedcba9876543210")
	t.Setenv("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS", "3")
	cfg, err := config.Load()
	require.NoError(t, err)
	service, _ := newAuthFixture(t)
	router := httpx.NewRouter(nil)
	mountAuthForTest(router, authcontroller.AuthControllerParams{AuthService: service, Limiter: authlimiter.NewLoginLimiter(startRedis(t), cfg.Auth, []byte(cfg.Auth.LoginLimiterKey))})
	for index := range 3 {
		require.Equal(t, http.StatusUnauthorized, attemptFrom(router, fmt.Sprintf("spray%d@example.com", index), "192.0.2.1:9000", "").Code)
	}
	blocked := attemptFrom(router, "another@example.com", "192.0.2.1:9000", "")
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	require.Equal(t, http.StatusUnauthorized, attemptFrom(router, "another@example.com", "192.0.2.2:9000", "").Code)
}
