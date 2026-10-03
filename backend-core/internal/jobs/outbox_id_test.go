package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestConsumerPreservesOutboxIDThroughDLQ(t *testing.T) {
	ctx := context.Background()
	rdb := startRedis(t)
	c := newConsumer(rdb, 1)
	require.NoError(t, c.EnsureGroup(ctx))
	const outboxID = "outbox-regression-id"
	require.NoError(t, rdb.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{FieldPayload: "bad", FieldOutboxID: outboxID}}).Err())
	require.NoError(t, c.Poll(ctx, func(_ context.Context, m Message) error {
		require.Equal(t, outboxID, m.OutboxID)
		return errors.New("poison")
	}))
	entries, err := rdb.XRange(ctx, testStream+DLQSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, outboxID, entries[0].Values[FieldOutboxID])
}
