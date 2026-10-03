package jobs

import (
	"context"
	"errors"
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

func newConsumer(rdb *redis.Client, maxDeliveries int) *Consumer {
	return &Consumer{Redis: rdb, Stream: testStream, Group: testGroup, Name: "w1", MaxDeliveries: maxDeliveries,
		VisibilityTimeout: time.Millisecond, BlockTimeout: 10 * time.Millisecond, BatchSize: 10, Log: quietLog}
}

func TestRelayPublishesOutboxRowsOnce(t *testing.T) {
	ctx := context.Background()
	pool, rdb := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, Enqueue(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Commit(ctx))

	relay := &Relay{Pool: pool, Redis: rdb, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	n, err = relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.EqualValues(t, 1, rdb.XLen(ctx, testStream).Val())
}

func TestEnqueueRolledBackIsNeverPublished(t *testing.T) {
	ctx := context.Background()
	pool, rdb := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, Enqueue(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Rollback(ctx))
	relay := &Relay{Pool: pool, Redis: rdb, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestConsumerAcksOnSuccess(t *testing.T) {
	ctx := context.Background()
	rdb := startRedis(t)
	c := newConsumer(rdb, 3)
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, rdb.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{FieldPayload: `{"a":1}`}}).Err())
	var got []string
	require.NoError(t, c.Poll(ctx, func(_ context.Context, m Message) error { got = append(got, string(m.Payload)); return nil }))
	require.Equal(t, []string{`{"a":1}`}, got)
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count)
}

func TestConsumerRedeliversThenSucceeds(t *testing.T) {
	ctx := context.Background()
	rdb := startRedis(t)
	c := newConsumer(rdb, 3)
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, rdb.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{FieldPayload: "x"}}).Err())
	calls := 0
	h := func(context.Context, Message) error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	}
	require.NoError(t, c.Poll(ctx, h))
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, c.Poll(ctx, h))
	require.Equal(t, 2, calls)
	require.Zero(t, rdb.XLen(ctx, testStream+DLQSuffix).Val())
}

func TestConsumerMovesPoisonMessageToDLQ(t *testing.T) {
	ctx := context.Background()
	rdb := startRedis(t)
	c := newConsumer(rdb, 2)
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, rdb.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{FieldPayload: "bad"}}).Err())
	calls := 0
	h := func(context.Context, Message) error { calls++; return errors.New("always broken") }
	for i := 0; i < 4; i++ {
		require.NoError(t, c.Poll(ctx, h))
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, 2, calls, "handler runs MaxDeliveries times, then never again")
	dlq, err := rdb.XRange(ctx, testStream+DLQSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, dlq, 1)
	require.Equal(t, "bad", dlq[0].Values[FieldPayload])
	require.Equal(t, "always broken", dlq[0].Values[FieldError])
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count)
}
