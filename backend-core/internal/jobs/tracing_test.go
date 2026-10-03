package jobs

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestHandlerRestoresTraceAndLogsWithoutPayload(t *testing.T) {
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
	rdb := startRedis(t)
	c := newConsumer(rdb, 3)
	var output bytes.Buffer
	log, err := logging.New(&output, "debug", "test", nil)
	require.NoError(t, err)
	c.Log = log
	require.NoError(t, c.EnsureGroup(context.Background()))
	require.NoError(t, rdb.XAdd(context.Background(), &redis.XAddArgs{Stream: testStream, Values: map[string]any{FieldPayload: "private-document", FieldTraceParent: carrier.Get(FieldTraceParent)}}).Err())
	require.NoError(t, c.Poll(context.Background(), func(ctx context.Context, _ Message) error {
		require.Equal(t, parent.SpanContext().TraceID(), trace.SpanContextFromContext(ctx).TraceID())
		return errors.New("private-document in handler failure")
	}))
	var handled sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "jobs.handle" {
			handled = span
		}
	}
	require.NotNil(t, handled)
	require.Equal(t, parent.SpanContext().SpanID(), handled.Parent().SpanID())
	require.Contains(t, output.String(), handled.SpanContext().TraceID().String())
	require.NotContains(t, output.String(), "private-document")
}
