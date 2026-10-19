package jobs

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestOutboxRelay_PublishesOutboxRowsOnce(t *testing.T) {
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, enqueueTestOutbox(ctx, tx, testStream, map[string]string{"file_id": "f1"}))
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
	require.NoError(t, enqueueTestOutbox(ctx, pool, testStream, map[string]string{"file_id": "other-file"}))
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

func TestOutboxRelay_PublishesRequestID(t *testing.T) {
	const origin = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	ctx := requestid.NewContext(tenant.WithID(context.Background(), tenant.PlatformID), origin)
	pool, redisClient := startPool(t), startRedis(t)
	require.NoError(t, enqueueTestOutbox(ctx, pool, testStream, map[string]string{"file_id": "f1"}))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Millisecond, Log: quietLog}
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	entries, err := redisClient.XRange(ctx, testStream, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, origin, entries[0].Values[StreamFieldRequestID])
	require.Equal(t, "", entries[0].Values[StreamFieldTraceParent])
}

func TestOutboxRelay_PublishOneOnlySelectedRow(t *testing.T) {
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	id, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "selected"})
	require.NoError(t, err)
	otherID, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "other"})
	require.NoError(t, err)
	require.NotEqual(t, id, otherID)
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, Log: quietLog}
	require.NoError(t, relay.PublishOne(ctx, id))
	require.NoError(t, relay.PublishOne(ctx, id))
	entries, err := redisClient.XRange(ctx, testStream, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, id, entries[0].Values[StreamFieldOutboxID])
}

func TestOutboxRelay_PublishOneSkipsLockedRow(t *testing.T) {
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	pool, redisClient := startPool(t), startRedis(t)
	id, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "locked"})
	require.NoError(t, err)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedID string
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM outbox WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID))
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, Log: quietLog}
	require.NoError(t, relay.PublishOne(ctx, id))
	require.Zero(t, redisClient.XLen(ctx, testStream).Val())
	require.NoError(t, tx.Commit(ctx))
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "the batch publishes the row PublishOne skipped")
	require.EqualValues(t, 1, redisClient.XLen(ctx, testStream).Val())
	require.NoError(t, relay.PublishOne(ctx, id))
	n, err = relay.PublishBatch(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.EqualValues(t, 1, redisClient.XLen(ctx, testStream).Val(), "the row is published exactly once")
}

func TestEnqueueOutbox_StoresNullTraceparentWithoutSpan(t *testing.T) {
	ctx := tenant.WithID(t.Context(), tenant.PlatformID)
	pool := startPool(t)
	id, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "untraced"})
	require.NoError(t, err)
	var isNull bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT traceparent IS NULL FROM outbox WHERE id=$1`, id).Scan(&isNull))
	require.True(t, isNull)
}

type cancelOnXAdd struct{ cancel context.CancelFunc }

func (cancelOnXAdd) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h cancelOnXAdd) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "xadd" {
			h.cancel()
		}
		return next(ctx, cmd)
	}
}
func (cancelOnXAdd) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestOutboxRelay_RunFinishesBatchInFlightWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(tenant.WithID(t.Context(), tenant.PlatformID))
	defer cancel()
	pool, redisClient := startPool(t), startRedis(t)
	id, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "draining"})
	require.NoError(t, err)
	redisClient.AddHook(cancelOnXAdd{cancel: cancel})
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, PollInterval: time.Hour, Log: quietLog}
	require.NoError(t, relay.Run(ctx))
	require.EqualValues(t, 1, redisClient.XLen(context.Background(), testStream).Val(), "the batch cancelled mid-flight must still complete")
	var published bool
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT published_at IS NOT NULL FROM outbox WHERE id=$1`, id).Scan(&published))
	require.True(t, published, "XADD and the published mark must stay in step, or the row is published twice")
}

func TestOutboxRelay_PublishOneCarriesTraceAndRequestID(t *testing.T) {
	// The global propagator is deliberately left alone: no code in this repo installs one, so enqueueing must not depend on it.
	require.Equal(t, 0, len(otel.GetTextMapPropagator().Fields()), "the default global propagator is a no-op")
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	const requestID = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	ctx := trace.ContextWithSpanContext(requestid.NewContext(tenant.WithID(t.Context(), tenant.PlatformID), requestID), span)
	pool, redisClient := startPool(t), startRedis(t)
	id, err := EnqueueOutbox(ctx, pool, testStream, map[string]string{"n": "traced"})
	require.NoError(t, err)
	relay := &OutboxRelay{Pool: pool, Redis: redisClient, BatchSize: 10, Log: quietLog}
	require.NoError(t, relay.PublishOne(ctx, id))
	entries, err := redisClient.XRange(ctx, testStream, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, requestID, entries[0].Values[StreamFieldRequestID])
	require.Equal(t, "00-01000000000000000000000000000000-0200000000000000-01", entries[0].Values[StreamFieldTraceParent])
	message := toMessage(testStream, entries[0], requestID, 1)
	require.Equal(t, entries[0].Values[StreamFieldTraceParent], message.TraceParent)
}
