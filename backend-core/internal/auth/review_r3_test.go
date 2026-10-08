package auth

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	authhandler "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authtoken "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/token"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestLogin_IPBudgetBlocksEmailSpray(t *testing.T) {
	t.Setenv("CORE_DATABASE_URL", "postgres://unused")
	t.Setenv("CORE_REDIS_URL", "redis://unused")
	t.Setenv("CORE_VAULT_MASTER_KEY", "unused")
	t.Setenv("CORE_SERVICE_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS", "3")
	cfg, err := config.Load()
	require.NoError(t, err)
	service, _ := newService(t)
	router := httpx.NewRouter(nil)
	mountAuthForTest(router, authhandler.AuthControllerParams{Service: service, Limiter: authlimiter.NewLoginLimiter(startRedis(t), cfg.Auth, []byte(cfg.ServiceToken))})
	for index := range 3 {
		require.Equal(t, http.StatusUnauthorized, attemptFrom(router, fmt.Sprintf("spray%d@example.com", index), "192.0.2.1:9000", "").Code)
	}
	blocked := attemptFrom(router, "another@example.com", "192.0.2.1:9000", "")
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))
	require.Equal(t, http.StatusUnauthorized, attemptFrom(router, "another@example.com", "192.0.2.2:9000", "").Code)
}

func TestAuthenticate_UsesDatabaseExpiryDespiteClockSkew(t *testing.T) {
	service, clock := newService(t)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	_, valid, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	_, expired, err := service.Login(ctx, ownerEmail, ownerPassword)
	require.NoError(t, err)
	clock.t = clock.t.Add(24 * time.Hour)
	_, _, err = service.Authenticate(ctx, valid.Token)
	require.NoError(t, err, "a valid DB session must survive an app clock ahead of DB")
	_, err = service.db.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, authtoken.Hash(expired.Token))
	require.NoError(t, err)
	clock.t = clock.t.Add(-48 * time.Hour)
	_, _, err = service.Authenticate(ctx, expired.Token)
	require.Equal(t, errors.CodeAuthSessionExpired, codeOf(t, err))
	var count int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash=$1`, authtoken.Hash(expired.Token)).Scan(&count))
	require.Zero(t, count)
}
