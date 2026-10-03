package postgres

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const postgresImage = "postgres:18.6-alpine"

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("thomas"), tcpostgres.WithUsername("thomas"), tcpostgres.WithPassword("thomas"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

func TestMigrateUpAndDown(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, url, migrations.FS, Up))

	pool, err := Open(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&n))

	require.NoError(t, Migrate(ctx, url, migrations.FS, Down))
	err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&n)
	require.Error(t, err)
}

func TestMigrateUpFailsOnBrokenMigration(t *testing.T) {
	url := startPostgres(t)
	broken := fstest.MapFS{
		"20260101000001_ok.sql":     {Data: []byte("-- +goose Up\nCREATE TABLE a (id int);\n-- +goose Down\nDROP TABLE a;\n")},
		"20260101000002_broken.sql": {Data: []byte("-- +goose Up\nCREATE TABLE b (id nonexistent_type);\n-- +goose Down\nDROP TABLE b;\n")},
	}
	err := Migrate(context.Background(), url, broken, Up)
	require.Error(t, err)
}

func TestOpenFailsFastOnBadURL(t *testing.T) {
	_, err := Open(context.Background(), "postgres://nobody:nothing@127.0.0.1:1/none?connect_timeout=1")
	require.Error(t, err)
}
