package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOutboxRelay_PublishesOutboxRowsOnce(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, EnqueueOutbox(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Commit(ctx))

	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	n, err = relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.EqualValues(t, 1, redisClient.XLen(ctx, testStream).Val())
}
