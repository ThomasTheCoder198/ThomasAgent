package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// seedReadyOrg inserts a provisioned organization row directly, so chat tests need no ragx provision call.
func seedReadyOrg(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO organizations (id, tenant_id, name, external_parsing, ragx_status, settings_synced)
		VALUES ($1, $2, $1, false, 'ready', true)`, id, tenant.PlatformID)
	require.NoError(t, err)
}

// seedUser inserts a platform user and returns its id; conversations reference users.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(), `INSERT INTO users (tenant_id, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		tenant.PlatformID, email).Scan(&id))
	return id
}

func orgContext(orgID string) context.Context {
	return tenant.WithID(context.Background(), orgID)
}

func conversationContext(orgID string, userID uuid.UUID) context.Context {
	return authmiddleware.WithUser(orgContext(orgID), authmodel.User{ID: userID})
}
