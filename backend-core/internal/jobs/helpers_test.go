package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/migrations"
)

const (
	testStream = "thomas.test.happened"
	testGroup  = "test-workers"
)

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	c, err := tcredis.Run(context.Background(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	url, err := c.ConnectionString(context.Background())
	require.NoError(t, err)
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	return redis.NewClient(opts)
}

func startPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:18.6-alpine", tcpostgres.WithDatabase("thomas"),
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

func newConsumer(redisClient *redis.Client, maxDeliveries int) *Consumer {
	return &Consumer{Redis: redisClient, Stream: testStream, GroupName: testGroup, ConsumerName: "w1", MaxDeliveries: maxDeliveries,
		VisibilityTimeout: time.Millisecond, BlockTimeout: 10 * time.Millisecond, BatchSize: 10, Log: quietLog}
}
