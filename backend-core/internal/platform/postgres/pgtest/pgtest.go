package pgtest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const image = "postgres:18.6-alpine"

func Start(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, image, tcpostgres.WithDatabase("thomas"),
		tcpostgres.WithUsername("thomas"), tcpostgres.WithPassword("thomas"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(ctx, url, migrations.FS, postgres.Up))
	pool, err := postgres.Open(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
