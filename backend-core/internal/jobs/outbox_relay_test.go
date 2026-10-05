package jobs

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestOutboxRelay_PublishesOutboxRowsOnce(t *testing.T) {
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
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

func TestOutboxRelayPreservesTenantForHandlerAndDeadLetter(t *testing.T) {
	pool, redisClient := startPool(t), startRedis(t)
	ctx := tenant.WithID(t.Context(), "other")
	require.NoError(t, EnqueueOutbox(ctx, pool, testStream, map[string]string{"file_id": "other-file"}))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	count, err := relay.PublishBatch(tenant.WithID(t.Context(), tenant.PlatformID))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	entries, err := redisClient.XRange(t.Context(), testStream, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "other", entries[0].Values["tenant_id"])
	consumer := newConsumer(redisClient, 1)
	require.NoError(t, consumer.EnsureGroup(t.Context()))
	handled := false
	require.NoError(t, consumer.PollOnce(tenant.WithID(t.Context(), tenant.PlatformID), func(ctx context.Context, msg Message) error {
		scope, err := tenant.ID(ctx)
		require.NoError(t, err)
		require.Equal(t, "other", scope)
		handled = true
		return stderrors.New("poison")
	}))
	require.True(t, handled)
	dlq, err := redisClient.XRange(t.Context(), testStream+DeadLetterSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, dlq, 1)
	require.Equal(t, "other", dlq[0].Values["tenant_id"])
}
