package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

// Roll back the retention index before the agents/chat migration; both directions run on the
// throwaway container of pgtest, never on a dev database.
func TestChatMigration_DownAndUpAgain(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := context.Background()
	url := pool.Config().ConnString()
	chatTables := []string{"agents", "conversations", "messages", "runs", "run_steps"}
	present := func(table string) bool {
		var found bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&found))
		return found
	}
	capabilitiesEdited := func() bool {
		var found bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_name = 'models' AND column_name = 'capabilities_edited')`).Scan(&found))
		return found
	}

	for _, table := range chatTables {
		require.True(t, present(table), table)
	}
	require.True(t, capabilitiesEdited())
	require.True(t, present("idx_conversations_updated_at"))
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Down))
	require.False(t, present("idx_conversations_updated_at"))

	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Down))
	for _, table := range chatTables {
		require.False(t, present(table), table)
	}
	require.False(t, capabilitiesEdited())

	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Up))
	for _, table := range chatTables {
		require.True(t, present(table), table)
	}
	require.True(t, capabilitiesEdited())
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Up))
	require.True(t, present("idx_conversations_updated_at"))
}
