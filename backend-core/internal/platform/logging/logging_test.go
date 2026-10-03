package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func testLogger(t *testing.T, w io.Writer, level, service string, otelHandler slog.Handler) *slog.Logger {
	t.Helper()
	logger, err := NewLogger(w, level, service, "test", otelHandler)
	require.NoError(t, err)
	return logger
}

func TestParseLevel_AcceptsKnownLevels(t *testing.T) {
	for name, want := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseLevel(name)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestParseLevel_RejectsUnknown(t *testing.T) {
	for _, name := range []string{"verbose", "", "INFO"} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseLevel(name)
			require.ErrorContains(t, err, "unknown log level")
		})
	}
}

func TestNewLogger_RejectsUnknownLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(&buf, "verbose", "service", "test", nil)
	require.ErrorContains(t, err, "unknown log level")
	require.Nil(t, logger)
	require.Empty(t, buf.String())
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestMultiHandler_ReturnsJSONWriteError(t *testing.T) {
	wantErr := errors.New("stdout write failed")
	var exported bytes.Buffer
	logger := testLogger(t, failingWriter{wantErr}, "info", "service", slog.NewJSONHandler(&exported, nil))
	err := logger.Handler().Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "event", 0))
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "event", lastLine(t, &exported)["msg"])
}

func TestMultiHandler_WritesJSONDespiteOTLPError(t *testing.T) {
	wantErr := errors.New("OTLP write failed")
	var local bytes.Buffer
	logger := testLogger(t, &local, "info", "service", slog.NewJSONHandler(failingWriter{wantErr}, nil))
	err := logger.Handler().Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "event", 0))
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "event", lastLine(t, &local)["msg"])
}

func TestMultiHandler_JoinsHandlerErrors(t *testing.T) {
	jsonErr := errors.New("stdout write failed")
	otlpErr := errors.New("OTLP write failed")
	logger := testLogger(t, failingWriter{jsonErr}, "info", "service", slog.NewJSONHandler(failingWriter{otlpErr}, nil))
	err := logger.Handler().Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "event", 0))
	require.ErrorIs(t, err, jsonErr)
	require.ErrorIs(t, err, otlpErr)
}

func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	var m map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &m))
	return m
}

func TestNewLogger_RedactsSecretKeysAndValues(t *testing.T) {
	var buf bytes.Buffer
	l := testLogger(t, &buf, "debug", "thomas-core", nil)
	l.Info("calling provider",
		"api_key", "sk-or-v1-abc",
		"Authorization", "Bearer xyz",
		"note", "token sk-live-123 leaked",
		"X-CSRF-Token", "abc",
		"input_tokens", 392,
		slog.Group("provider", "password", "hunter2", "name", "openrouter"),
	)
	m := lastLine(t, &buf)
	require.Equal(t, RedactedPlaceholder, m["X-CSRF-Token"])
	require.EqualValues(t, 392, m["input_tokens"], "usage counters must stay visible")
	require.Equal(t, RedactedPlaceholder, m["api_key"])
	require.Equal(t, RedactedPlaceholder, m["Authorization"])
	require.NotContains(t, m["note"], "sk-live-123")
	group := m["provider"].(map[string]any)
	require.Equal(t, RedactedPlaceholder, group["password"])
	require.Equal(t, "openrouter", group["name"])
}

func TestNewLogger_AddsServiceAndTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	l := testLogger(t, &buf, "info", "thomas-core", nil)
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(),
		trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
	l.InfoContext(ctx, "hello")
	m := lastLine(t, &buf)
	require.Equal(t, "thomas-core", m["service"])
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", m["trace_id"])
	require.Equal(t, "00f067aa0ba902b7", m["span_id"])
}

func TestNewLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	l := testLogger(t, &buf, "warn", "thomas-core", nil)
	l.Info("hidden")
	require.Empty(t, buf.String())
}

func TestNewLogger_ExportRedactsBoundAndGroupedAttributesAndHonorsLevel(t *testing.T) {
	var local, exported bytes.Buffer
	sink := slog.NewJSONHandler(&exported, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := testLogger(t, &local, "warn", "thomas-core", sink).With("api_key", "bound-secret").WithGroup("provider").With("password", "group-secret")
	l.Info("hidden")
	require.Empty(t, exported.String())
	l.Warn("token sk-live-message", "token", "record-secret", slog.Group("nested", "password", "nested-secret"), "details", map[string]any{"password": "map-secret"})
	for _, buf := range []*bytes.Buffer{&local, &exported} {
		require.NotContains(t, buf.String(), "bound-secret")
		require.NotContains(t, buf.String(), "group-secret")
		require.NotContains(t, buf.String(), "record-secret")
		require.NotContains(t, buf.String(), "nested-secret")
		require.NotContains(t, buf.String(), "map-secret")
		require.NotContains(t, buf.String(), "sk-live-message")
	}
}

func TestNewLogger_RedactsSecretAncestorGroupsInBothSinks(t *testing.T) {
	cases := map[string]func(*slog.Logger){
		"direct":       func(l *slog.Logger) { l.Info("event", slog.Group("authorization", "value", "plain-secret")) },
		"bound-direct": func(l *slog.Logger) { l.With(slog.Group("authorization", "value", "plain-secret")).Info("event") },
		"derived":      func(l *slog.Logger) { l.WithGroup("authorization").Info("event", "value", "plain-secret") },
		"derived-bound-nested": func(l *slog.Logger) {
			l.WithGroup("authorization").WithGroup("nested").With("value", "plain-secret").Info("event", "other", "plain-secret")
		},
	}
	for name, emit := range cases {
		t.Run(name, func(t *testing.T) {
			var local, exported bytes.Buffer
			l := testLogger(t, &local, "info", "service", slog.NewJSONHandler(&exported, nil))
			emit(l)
			for _, buf := range []*bytes.Buffer{&local, &exported} {
				require.NotContains(t, buf.String(), "plain-secret")
				require.Contains(t, buf.String(), RedactedPlaceholder)
			}
		})
	}
}

func TestNewLogger_RedactsNestedContainersInBothSinks(t *testing.T) {
	var local, exported bytes.Buffer
	l := testLogger(t, &local, "info", "service", slog.NewJSONHandler(&exported, nil))
	details := map[string]any{
		"providers": []map[string]any{{"api_key": "plain-secret", "input_tokens": 392}},
		"headers":   map[string]string{"Authorization": "plain-secret", "name": "visible"},
		"nested":    [][]map[string]string{{{"password": "plain-secret"}}},
		"values":    []any{[]string{"Bearer plain-secret"}},
		"token":     []map[string]string{{"value": "plain-secret"}},
	}
	l.With("details", details).Info("event")
	for _, buf := range []*bytes.Buffer{&local, &exported} {
		require.NotContains(t, buf.String(), "plain-secret")
		m := lastLine(t, buf)["details"].(map[string]any)
		provider := m["providers"].([]any)[0].(map[string]any)
		require.Equal(t, RedactedPlaceholder, provider["api_key"])
		require.EqualValues(t, 392, provider["input_tokens"])
		require.Equal(t, "visible", m["headers"].(map[string]any)["name"])
		require.Equal(t, RedactedPlaceholder, m["token"])
	}
	require.Equal(t, "plain-secret", details["headers"].(map[string]string)["Authorization"])
}
