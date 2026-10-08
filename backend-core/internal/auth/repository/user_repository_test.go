package repository

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const ownerEmail = "thomas@example.com"

func TestUserRepo_RejectsMissingTenant(t *testing.T) {
	users := NewUserRepo(pgtest.Start(t))
	_, err := users.CountUsers(t.Context())
	require.ErrorIs(t, err, errors.ErrMissingTenant, "absent tenant must never select the platform")
	_, err = users.CreateUser(t.Context(), ownerEmail, "hash")
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	_, _, err = users.FindUserByEmail(t.Context(), ownerEmail)
	require.ErrorIs(t, err, errors.ErrMissingTenant)
}

func TestUserRepo_UsesTrustedTenantContext(t *testing.T) {
	pool := pgtest.Start(t)
	users := NewUserRepo(pool)
	platform, err := users.CreateUser(tenant.WithID(t.Context(), tenant.PlatformID), ownerEmail, "platform-hash")
	require.NoError(t, err)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := users.CreateUser(otherCtx, ownerEmail, "other-hash")
	require.NoError(t, err)
	require.NotEqual(t, platform.ID, other.ID)
	got, hash, err := users.FindUserByEmail(otherCtx, ownerEmail)
	require.NoError(t, err)
	require.Equal(t, other.ID, got.ID)
	require.Equal(t, "other-hash", hash)
	_, err = pool.Exec(t.Context(), `INSERT INTO users (tenant_id,email,password_hash) VALUES ('other','second@example.com','hash')`)
	require.NoError(t, err)
	count, err := users.CountUsers(otherCtx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestUserRepo_FindUserByEmailScopesSameEmailByTenant(t *testing.T) {
	pool := pgtest.Start(t)
	users := NewUserRepo(pool)
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	platform, err := users.CreateUser(ctx, ownerEmail, "platform-hash")
	require.NoError(t, err)
	otherID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, tenant_id, email, password_hash) VALUES ($1, $2, $3, $4)`, otherID, "other", ownerEmail, "other-hash")
	require.NoError(t, err)
	user, _, err := users.FindUserByEmail(ctx, ownerEmail)
	require.NoError(t, err)
	require.Equal(t, platform.ID, user.ID)
	count, err := users.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
