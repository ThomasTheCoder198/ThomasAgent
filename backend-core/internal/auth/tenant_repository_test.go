package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestAuthRepositoryRejectsMissingTenant(t *testing.T) {
	repo := NewRepository(pgtest.Start(t))
	_, err := repo.CountUsers(t.Context())
	require.ErrorIs(t, err, tenant.ErrMissingTenant, "absent tenant must never select the platform")
	_, err = repo.CreateUser(t.Context(), ownerEmail, "hash")
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
	_, _, err = repo.FindUserByEmail(t.Context(), ownerEmail)
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
	require.ErrorIs(t, repo.CreateSession(t.Context(), User{}.ID, []byte("token"), "csrf", time.Now().Add(time.Hour)), tenant.ErrMissingTenant)
	_, _, err = repo.FindSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
	err = repo.DeleteSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
	_, err = repo.PurgeExpiredSessions(t.Context())
	require.ErrorIs(t, err, tenant.ErrMissingTenant)
}

func TestRequireSessionDerivesTenantFromSessionRow(t *testing.T) {
	svc, _ := newService(t)
	platformCtx := tenant.WithID(t.Context(), tenant.PlatformID)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := svc.repo.CreateUser(otherCtx, "other@example.com", "hash")
	require.NoError(t, err)
	token := "other-session-token"
	require.NoError(t, svc.repo.CreateSession(otherCtx, other.ID, hashToken(token), "csrf", time.Now().Add(time.Hour)))
	handler := RequireSession(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, err := tenant.ID(r.Context())
		require.NoError(t, err)
		require.Equal(t, "other", scope)
		_, _, err = svc.repo.FindUserByEmail(r.Context(), ownerEmail)
		require.ErrorIs(t, err, errNotFound, "a tenant A session must not read tenant B's user")
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/tenant-data", nil).WithContext(platformCtx)
	request.AddCookie(&http.Cookie{Name: CookieSession, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestFindSessionThrottlesLastSeenAndSkipsExpiredRows(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewRepository(pool)
	ctx := tenant.WithID(t.Context(), "other")
	user, err := repo.CreateUser(ctx, "session@example.com", "hash")
	require.NoError(t, err)
	token := []byte("throttled-session-token")
	require.NoError(t, repo.CreateSession(ctx, user.ID, token, "csrf", time.Now().Add(time.Hour)))
	var before, after time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = repo.FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after, "requests within the touch interval must not write")
	_, err = pool.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 hour', last_seen_at='2000-01-01' WHERE token_hash=$1`, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = repo.FindSession(ctx, token)
	require.NoError(t, err, "expiry remains distinguishable by authentication")
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after, "expired rows must not be touched")
}

func TestFindSessionHonorsConfiguredTouchInterval(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(t.Context(), "other")
	repo := NewRepositoryWithSessionTouchInterval(pool, 2*time.Hour)
	user, err := repo.CreateUser(ctx, "configured@example.com", "hash")
	require.NoError(t, err)
	token := []byte("configured-touch-token")
	require.NoError(t, repo.CreateSession(ctx, user.ID, token, "csrf", time.Now().Add(time.Hour)))
	_, err = pool.Exec(ctx, `UPDATE sessions SET last_seen_at=now()-interval '1 hour' WHERE token_hash=$1`, token)
	require.NoError(t, err)
	var before, after time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = repo.FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after)
	_, _, err = NewRepositoryWithSessionTouchInterval(pool, 30*time.Minute).FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.True(t, after.After(before))
}
