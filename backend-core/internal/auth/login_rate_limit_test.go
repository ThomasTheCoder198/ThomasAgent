package auth

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
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
	svc, _ := newTestService(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger))
	mountAuthForTest(router, ControllerParams{Service: svc, Limiter: newTestLimiter(startRedis(t), 1), TrustedProxyHeader: "X-Forwarded-For", Logger: logger})
	require.Equal(t, 401, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.1").Code)
	require.Equal(t, 429, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "198.51.100.2").Code)
}

func TestLogin_EmailBudgetBlocksRotatingIPs(t *testing.T) {
	h, _ := newServer(t, 2)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.1:9999", "").Code)
	require.Equal(t, 401, attemptFrom(h, ownerEmail, "192.0.2.2:9999", "").Code)
	require.Equal(t, 429, attemptFrom(h, ownerEmail, "192.0.2.3:9999", "").Code)
}

func TestFailedLogin_DoesNotWriteUnboundedAuditRows(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	for range 3 {
		_, _, err := svc.Login(ctx, ownerEmail, "incorrect-password")
		require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, err))
	}
	var count int
	require.NoError(t, svc.db.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='auth.login.failed'`).Scan(&count))
	require.Zero(t, count, "ordinary failed attempts must not persist an audit row")
	svc.db.inner = failingExecDB{DBTX: svc.db.inner, statement: "INSERT INTO audit_events", failure: stderrors.New("audit unavailable")}
	_, _, err := svc.Login(ctx, ownerEmail, "incorrect-password")
	require.Equal(t, errors.CodeAuthInvalidCredentials, codeOf(t, err))
}

type scriptExpiryFailureHook struct{ failure error }

func (h scriptExpiryFailureHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h scriptExpiryFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h scriptExpiryFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "evalsha" || cmd.Name() == "eval" {
			return h.failure
		}
		return next(ctx, cmd)
	}
}

func TestLogin_IPBudgetBlocksEmailSpray(t *testing.T) {
	t.Setenv("CORE_DATABASE_URL", "postgres://unused")
	t.Setenv("CORE_REDIS_URL", "redis://unused")
	t.Setenv("CORE_VAULT_MASTER_KEY", "unused")
	t.Setenv("CORE_SERVICE_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS", "3")
	cfg, err := config.Load()
	require.NoError(t, err)
	service, _ := newTestService(t)
	router := httpx.NewRouter(nil)
	mountAuthForTest(router, ControllerParams{Service: service, Limiter: NewLoginLimiter(startRedis(t), cfg.Auth, []byte(cfg.ServiceToken))})
	for index := range 3 {
		require.Equal(t, http.StatusUnauthorized, attemptFrom(router, fmt.Sprintf("spray%d@example.com", index), "192.0.2.1:9000", "").Code)
	}
	blocked := attemptFrom(router, "another@example.com", "192.0.2.1:9000", "")
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	require.Equal(t, http.StatusUnauthorized, attemptFrom(router, "another@example.com", "192.0.2.2:9000", "").Code)
}
