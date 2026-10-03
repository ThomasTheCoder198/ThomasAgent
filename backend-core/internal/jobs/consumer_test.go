package jobs

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
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
	require.NoError(t, c.EnsureGroup(ctx))
	require.NoError(t, redisClient.XAdd(ctx, &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "x"}}).Err())
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

func TestConsumer_RestoresTraceAndLogsWithoutPayload(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	ctx, parent := provider.Tracer("test").Start(context.Background(), "test.enqueue")
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	parent.End()
	redisClient := startRedis(t)
	c := newConsumer(redisClient, 3)
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "debug", "test", "test", nil)
	require.NoError(t, err)
	c.Log = log
	require.NoError(t, c.EnsureGroup(context.Background()))
	require.NoError(t, redisClient.XAdd(context.Background(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{StreamFieldPayload: "private-document", StreamFieldTraceParent: carrier.Get(StreamFieldTraceParent)}}).Err())
	require.NoError(t, c.PollOnce(context.Background(), func(ctx context.Context, _ Message) error {
		require.Equal(t, parent.SpanContext().TraceID(), trace.SpanContextFromContext(ctx).TraceID())
		return errors.New("private-document in handler failure")
	}))
	var handled sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "core.jobs.handle" {
			handled = span
		}
	}
	require.NotNil(t, handled)
	t.Logf("emitted_span=%s trace_id=%s parent_span_id=%s", handled.Name(), handled.SpanContext().TraceID(), handled.Parent().SpanID())
	require.Equal(t, parent.SpanContext().SpanID(), handled.Parent().SpanID())
	require.Contains(t, output.String(), handled.SpanContext().TraceID().String())
	require.NotContains(t, output.String(), "private-document")
}
