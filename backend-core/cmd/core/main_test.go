package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestDatabaseFailureBoundaries(t *testing.T) {
	for _, operation := range []string{"migrate", "open"} {
		t.Run(operation, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			var output bytes.Buffer
			a := &app{
				cfg:      config.Config{DatabaseURL: "postgres://nobody:secret-password@127.0.0.1:1/none?connect_timeout=1"},
				log:      logging.New(&output, "info", "core-test", nil),
				dbTracer: provider.Tracer("core-test"),
			}
			var err error
			if operation == "migrate" {
				err = a.migrate(context.Background(), nil)
			} else {
				_, err = a.openDatabase(context.Background())
			}
			require.Error(t, err)
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, "postgres."+operation, spans[0].Name())
			require.Equal(t, codes.Error, spans[0].Status().Code)
			var record map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &record))
			require.Equal(t, "ERROR", record["level"])
			require.Equal(t, spans[0].SpanContext().TraceID().String(), record["trace_id"])
			require.Equal(t, spans[0].SpanContext().SpanID().String(), record["span_id"])
			require.Equal(t, "INTERNAL_ERROR", record["error_code"])
			require.NotContains(t, output.String(), "secret-password")
			require.NotContains(t, output.String(), "postgres://")
		})
	}
}
