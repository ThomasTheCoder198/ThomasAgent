package jobs

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

func TestConsumer_AcksOnSuccess(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 3)
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: `{"a":1}`}}).Err())
	var got []string
	require.NoError(t, c.PollOnce(ctx, func(_ context.Context, m Message) error { got = append(got, string(m.Payload)); return nil }))
	require.Equal(t, []string{`{"a":1}`}, got)
	pending, err := redisClient.XPending(ctx, testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count)
}

func TestConsumer_RedeliversThenSucceeds(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 3)
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "debug", "test", "test", nil)
	require.NoError(t, err)
	c.Log = log
	const outboxID = "retry-outbox-id"
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "x", StreamFieldOutboxID: outboxID}}).Err())
	calls := 0
	h := func(context.Context, Message) error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	}
	require.NoError(t, c.PollOnce(ctx, h))
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, c.PollOnce(ctx, h))
	require.Contains(t, output.String(), `"msg":"job failed, will retry"`)
	require.Contains(t, output.String(), `"outbox_id":"`+outboxID+`"`)
	require.Contains(t, output.String(), `"stream":"`+testStream+`"`)
	require.Equal(t, 2, calls)
	require.Zero(t, redisClient.XLen(ctx, testStream+DeadLetterSuffix).Val())
}

func TestConsumer_MovesPoisonMessageToDLQ(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 2)
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "bad"}}).Err())
	calls := 0
	h := func(context.Context, Message) error { calls++; return errors.New("always broken") }
	for i := 0; i < 4; i++ {
		require.NoError(t, c.PollOnce(ctx, h))
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, 2, calls, "handler runs MaxDeliveries times, then never again")
	dlq, err := redisClient.XRange(ctx, testStream+DeadLetterSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, dlq, 1)
	require.Equal(t, "bad", dlq[0].Values[StreamFieldPayload])
	require.Equal(t, "always broken", dlq[0].Values[StreamFieldError])
	pending, err := redisClient.XPending(ctx, testStream, testGroup).Result()
	require.NoError(t, err)
	require.Zero(t, pending.Count)
}

func TestConsumer_PreservesOutboxIDThroughDLQ(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 1)
	require.NoError(t, c.EnsureGroup(ctx))
	const outboxID = "outbox-regression-id"
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "bad", StreamFieldOutboxID: outboxID}}).Err())
	require.NoError(t, c.PollOnce(ctx, func(_ context.Context, m Message) error {
		require.Equal(t, outboxID, m.OutboxID)
		return errors.New("poison")
	}))
	entries, err := redisClient.XRange(ctx, testStream+DeadLetterSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, outboxID, entries[0].Values[StreamFieldOutboxID])
}

func TestConsumer_RestoresRequestIDAndLogsWithoutPayload(t *testing.T) {
	const outboxID = "correlated-dlq-outbox-id"
	const origin = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 1)
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "debug", "test", "test", nil)
	require.NoError(t, err)
	c.Log = log
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{
		StreamFieldPayload: "private-document", StreamFieldRequestID: origin, StreamFieldOutboxID: outboxID,
	}}).Err())
	require.NoError(t, c.PollOnce(ctx, func(ctx context.Context, m Message) error {
		require.Equal(t, origin, requestid.FromContext(ctx))
		require.Equal(t, origin, m.RequestID)
		return errors.New("private-document in handler failure")
	}))
	entries, err := redisClient.XRange(ctx, testStream+DeadLetterSuffix, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, origin, entries[0].Values[StreamFieldRequestID])
	require.Contains(t, output.String(), `"request_id":"`+origin+`"`)
	require.Contains(t, output.String(), `"duration_ms"`)
	require.Contains(t, output.String(), `"outbox_id":"`+outboxID+`"`)
	require.Contains(t, output.String(), `"stream":"`+testStream+`"`)
	require.NotContains(t, output.String(), "private-document")
}

func TestConsumer_ReplacesMalformedRequestID(t *testing.T) {
	ctx := context.Background()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 3)
	require.NoError(t, c.EnsureGroup(ctx))
	for _, values := range []map[string]any{
		{StreamFieldPayload: "a", StreamFieldRequestID: "forged\nid"},
		{StreamFieldPayload: "b"},
	} {
		require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: values}).Err())
	}
	var seen []string
	require.NoError(t, c.PollOnce(ctx, func(ctx context.Context, m Message) error {
		seen = append(seen, requestid.FromContext(ctx))
		require.Equal(t, requestid.FromContext(ctx), m.RequestID)
		return nil
	}))
	require.Len(t, seen, 2)
	for _, id := range seen {
		require.True(t, requestid.IsValid(id), id)
		require.True(t, strings.HasPrefix(id, requestid.JobPrefix), id)
	}
}
