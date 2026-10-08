package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestRepository_RejectsMissingTenant(t *testing.T) {
	repo := NewRepository(pgtest.Start(t))
	_, err := repo.CountUsers(t.Context())
	require.ErrorIs(t, err, errors.ErrMissingTenant, "absent tenant must never select the platform")
	_, err = repo.CreateUser(t.Context(), ownerEmail, "hash")
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	_, _, err = repo.FindUserByEmail(t.Context(), ownerEmail)
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	require.ErrorIs(t, repo.CreateSession(t.Context(), User{}.ID, []byte("token"), "csrf", time.Now().Add(time.Hour)), errors.ErrMissingTenant)
	_, _, err = repo.FindSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	err = repo.DeleteSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	_, err = repo.PurgeExpiredSessions(t.Context())
	require.ErrorIs(t, err, errors.ErrMissingTenant)
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

func TestRepository_UsesTrustedTenantContext(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewRepository(pool)
	platform, err := repo.CreateUser(tenant.WithID(t.Context(), tenant.PlatformID), ownerEmail, "platform-hash")
	require.NoError(t, err)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := repo.CreateUser(otherCtx, ownerEmail, "other-hash")
	require.NoError(t, err)
	require.NotEqual(t, platform.ID, other.ID)
	got, hash, err := repo.FindUserByEmail(otherCtx, ownerEmail)
	require.NoError(t, err)
	require.Equal(t, other.ID, got.ID)
	require.Equal(t, "other-hash", hash)
	require.NoError(t, repo.CreateSession(otherCtx, other.ID, []byte("other-token"), "csrf", time.Now().Add(time.Hour)))
	_, _, err = repo.FindSession(tenant.WithID(t.Context(), tenant.PlatformID), []byte("other-token"))
	require.ErrorIs(t, err, errors.ErrNotFound)
	_, _, err = repo.FindSession(otherCtx, []byte("other-token"))
	require.NoError(t, err)
	_, err = repo.db.Exec(tenant.WithID(t.Context(), tenant.PlatformID), `INSERT INTO users (tenant_id,email,password_hash) VALUES ('other','second@example.com','hash')`)
	require.NoError(t, err)
	count, err := repo.CountUsers(otherCtx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestFindUserByEmailScopesSameEmailByTenant(t *testing.T) {
	service, _ := newTestService(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	otherID := uuid.New()
	_, err := service.repository.db.Exec(ctx, `INSERT INTO users (id, tenant_id, email, password_hash) VALUES ($1, $2, $3, $4)`, otherID, "other", ownerEmail, "other-hash")
	require.NoError(t, err)
	user, _, err := service.repository.FindUserByEmail(ctx, ownerEmail)
	require.NoError(t, err)
	require.NotEqual(t, otherID, user.ID)
	count, err := service.repository.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
