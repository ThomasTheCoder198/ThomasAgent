package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestSessionRepo_RejectsMissingTenant(t *testing.T) {
	sessions := NewSessionRepo(pgtest.Start(t), 0)
	require.ErrorIs(t, sessions.CreateSession(t.Context(), uuid.Nil, []byte("token"), "csrf", time.Now().Add(time.Hour)), errors.ErrMissingTenant)
	_, _, err := sessions.FindSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	err = sessions.DeleteSession(t.Context(), []byte("token"))
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	_, err = sessions.PurgeExpiredSessions(t.Context())
	require.ErrorIs(t, err, errors.ErrMissingTenant)
}

func TestSessionRepo_FindSessionIsScopedToTenant(t *testing.T) {
	pool := pgtest.Start(t)
	sessions := NewSessionRepo(pool, 0)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := NewUserRepo(pool).CreateUser(otherCtx, ownerEmail, "other-hash")
	require.NoError(t, err)
	require.NoError(t, sessions.CreateSession(otherCtx, other.ID, []byte("other-token"), "csrf", time.Now().Add(time.Hour)))
	_, _, err = sessions.FindSession(tenant.WithID(t.Context(), tenant.PlatformID), []byte("other-token"))
	require.ErrorIs(t, err, errors.ErrNotFound)
	_, _, err = sessions.FindSession(otherCtx, []byte("other-token"))
	require.NoError(t, err)
	_, session, err := sessions.FindSessionAcrossTenants(t.Context(), []byte("other-token"))
	require.NoError(t, err)
	require.Equal(t, "other", session.TenantID, "the session row, not the caller, decides the tenant")
}

func TestSessionRepo_FindSessionThrottlesLastSeenAndSkipsExpiredRows(t *testing.T) {
	pool := pgtest.Start(t)
	sessions := NewSessionRepo(pool, 0)
	ctx := tenant.WithID(t.Context(), "other")
	user, err := NewUserRepo(pool).CreateUser(ctx, "session@example.com", "hash")
	require.NoError(t, err)
	token := []byte("throttled-session-token")
	require.NoError(t, sessions.CreateSession(ctx, user.ID, token, "csrf", time.Now().Add(time.Hour)))
	var before, after time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = sessions.FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after, "requests within the touch interval must not write")
	_, err = pool.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 hour', last_seen_at='2000-01-01' WHERE token_hash=$1`, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = sessions.FindSession(ctx, token)
	require.NoError(t, err, "expiry remains distinguishable by authentication")
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after, "expired rows must not be touched")
}

func TestSessionRepo_FindSessionHonorsConfiguredTouchInterval(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(t.Context(), "other")
	user, err := NewUserRepo(pool).CreateUser(ctx, "configured@example.com", "hash")
	require.NoError(t, err)
	token := []byte("configured-touch-token")
	require.NoError(t, NewSessionRepo(pool, 2*time.Hour).CreateSession(ctx, user.ID, token, "csrf", time.Now().Add(time.Hour)))
	_, err = pool.Exec(ctx, `UPDATE sessions SET last_seen_at=now()-interval '1 hour' WHERE token_hash=$1`, token)
	require.NoError(t, err)
	var before, after time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&before))
	_, _, err = NewSessionRepo(pool, 2*time.Hour).FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.Equal(t, before, after)
	_, _, err = NewSessionRepo(pool, 30*time.Minute).FindSession(ctx, token)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE token_hash=$1`, token).Scan(&after))
	require.True(t, after.After(before))
}
