package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEnqueueOutbox_RolledBackIsNeverPublished(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, EnqueueOutbox(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
	require.NoError(t, tx.Rollback(ctx))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}
