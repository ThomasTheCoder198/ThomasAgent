package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

// Down rolls back the run model columns, then the run lease columns, then the outbox traceparent column, the retention index, and the agents/chat migration;
// both directions run on the throwaway container of pgtest, never on a dev database.
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

	traceparentColumn := func() bool {
		var found bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_name = 'outbox' AND column_name = 'traceparent')`).Scan(&found))
		return found
	}

	runLeaseColumns := func() int {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'runs'
			AND column_name IN ('lease_owner', 'lease_token', 'lease_until', 'request_id')`).Scan(&count))
		return count
	}

	runModelColumns := func() int {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'runs'
			AND column_name IN ('requested_model_id', 'locale')`).Scan(&count))
		return count
	}

	for _, table := range chatTables {
		require.True(t, present(table), table)
	}
	require.True(t, capabilitiesEdited())
	require.True(t, present("idx_conversations_updated_at"))
	require.True(t, traceparentColumn())
	require.True(t, runLeaseColumns() == 4)
	require.Equal(t, 2, runModelColumns())
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Down))
	require.Zero(t, runModelColumns(), "the run model migration is the latest")
	require.Equal(t, 4, runLeaseColumns())
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Down))
	require.Zero(t, runLeaseColumns(), "the run lease migration is next")
	require.True(t, traceparentColumn())
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Down))
	require.False(t, traceparentColumn())
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
	require.Equal(t, 4, runLeaseColumns(), "Up again restores the run lease columns")
	require.Equal(t, 2, runModelColumns(), "Up again restores the run model columns")
}
