package jobs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

func TestOutboxTraceparentMigration_UpDownUp(t *testing.T) {
	container, err := tcpostgres.Run(t.Context(), "postgres:18.6-alpine", tcpostgres.WithDatabase("thomas"), tcpostgres.WithUsername("thomas"), tcpostgres.WithPassword("thomas"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	url, err := container.ConnectionString(t.Context(), "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(t.Context(), url, migrations.FS, postgres.Up))
	pool, err := postgres.Open(t.Context(), url)
	require.NoError(t, err)
	defer pool.Close()
	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_name='outbox' AND column_name='traceparent'`).Scan(&count))
	require.Equal(t, 1, count)
	// The run lease migration sits on top of it, so it is rolled back first.
	require.NoError(t, postgres.Migrate(t.Context(), url, migrations.FS, postgres.Down))
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_name='outbox' AND column_name='traceparent'`).Scan(&count))
	require.Equal(t, 1, count, "the first Down removes only the run lease columns")
	require.NoError(t, postgres.Migrate(t.Context(), url, migrations.FS, postgres.Down))
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_name='outbox' AND column_name='traceparent'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, postgres.Migrate(t.Context(), url, migrations.FS, postgres.Up))
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_name='outbox' AND column_name='traceparent'`).Scan(&count))
	require.Equal(t, 1, count)
}
