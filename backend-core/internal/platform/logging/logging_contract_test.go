package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func contractContext(t *testing.T) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
}

func TestNewLogger_JSONContract(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
	require.NoError(t, err)
	timestamp := time.Date(2026, time.October, 3, 14, 15, 16, 123456789, time.FixedZone("fixture", 7*60*60))
	record := slog.NewRecord(timestamp, slog.LevelInfo, "event", 0)
	record.AddAttrs(slog.Group("details", slog.Time("time", timestamp)))
	require.NoError(t, logger.Handler().Handle(contractContext(t), record))
	var entry map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	require.Equal(t, "2026-10-03T07:15:16.123456789Z", entry["ts"])
	parsedTimestamp, err := time.Parse(time.RFC3339Nano, entry["ts"].(string))
	require.NoError(t, err)
	require.Equal(t, timestamp.UTC(), parsedTimestamp)
	require.NotContains(t, entry, "time")
	require.Equal(t, "INFO", entry["level"])
	require.Equal(t, "event", entry["msg"])
	require.Equal(t, "core-test", entry["service"])
	require.Equal(t, "test", entry["env"])
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", entry["trace_id"])
	require.Equal(t, "00f067aa0ba902b7", entry["span_id"])
	require.NotContains(t, entry, "request_id")
	require.Equal(t, timestamp.Format(time.RFC3339Nano), entry["details"].(map[string]any)["time"])
}

type recordingExporter struct{ records []sdklog.Record }

func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

func (*recordingExporter) Shutdown(context.Context) error   { return nil }
func (*recordingExporter) ForceFlush(context.Context) error { return nil }

func TestNewLogger_OTLPContract(t *testing.T) {
	exporter := &recordingExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	var output bytes.Buffer
	logger, err := logging.NewLogger(&output, "info", "core-test", "test", otelslog.NewHandler("core-test", otelslog.WithLoggerProvider(provider)))
	require.NoError(t, err)
	router := httpx.NewRouter(nil)
	router.Get("/log", func(w http.ResponseWriter, req *http.Request) {
		logger.With("api_key", "bound-secret").InfoContext(req.Context(), "handler event", "Authorization", "record-secret")
		httpx.WriteSuccess(w, req, http.StatusOK, nil)
	})
	req := httptest.NewRequestWithContext(contractContext(t), http.MethodGet, "/log", nil)
	req.Header.Set("X-Request-Id", "otlp-request")
	router.ServeHTTP(httptest.NewRecorder(), req)
	require.Len(t, exporter.records, 1)
	record := exporter.records[0]
	attributes := map[string]string{}
	record.WalkAttributes(func(value attribute.KeyValue) bool {
		attributes[string(value.Key)] = value.Value.AsString()
		return true
	})
	require.Equal(t, "test", attributes["env"])
	require.Equal(t, "core-test", attributes["service"])
	require.Equal(t, "otlp-request", attributes["request_id"])
	require.Equal(t, logging.RedactedPlaceholder, attributes["api_key"])
	require.Equal(t, logging.RedactedPlaceholder, attributes["Authorization"])
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", record.TraceID().String())
	require.Equal(t, "00f067aa0ba902b7", record.SpanID().String())
	require.Equal(t, record.TraceID().String(), attributes["trace_id"])
	require.Equal(t, record.SpanID().String(), attributes["span_id"])
	require.Equal(t, "INFO", record.SeverityText())
	require.Equal(t, "handler event", record.Body().AsString())
	require.False(t, record.Timestamp().IsZero())
}
