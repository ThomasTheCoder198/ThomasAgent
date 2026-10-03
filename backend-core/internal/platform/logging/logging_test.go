package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	var m map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &m))
	return m
}

func TestRedactsSecretKeysAndValues(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "debug", "thomas-core", nil)
	l.Info("calling provider",
		"api_key", "sk-or-v1-abc",
		"Authorization", "Bearer xyz",
		"note", "token sk-live-123 leaked",
		"X-CSRF-Token", "abc",
		"input_tokens", 392,
		slog.Group("provider", "password", "hunter2", "name", "openrouter"),
	)
	m := lastLine(t, &buf)
	require.Equal(t, Redacted, m["X-CSRF-Token"])
	require.EqualValues(t, 392, m["input_tokens"], "usage counters must stay visible")
	require.Equal(t, Redacted, m["api_key"])
	require.Equal(t, Redacted, m["Authorization"])
	require.NotContains(t, m["note"], "sk-live-123")
	group := m["provider"].(map[string]any)
	require.Equal(t, Redacted, group["password"])
	require.Equal(t, "openrouter", group["name"])
}

func TestAddsServiceAndTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "info", "thomas-core", nil)
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

func TestRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "warn", "thomas-core", nil)
	l.Info("hidden")
	require.Empty(t, buf.String())
}

func TestExportRedactsBoundAndGroupedAttributesAndHonorsLevel(t *testing.T) {
	var local, exported bytes.Buffer
	sink := slog.NewJSONHandler(&exported, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := New(&local, "warn", "thomas-core", sink).With("api_key", "bound-secret").WithGroup("provider").With("password", "group-secret")
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

func TestRedactsSecretAncestorGroupsInBothSinks(t *testing.T) {
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
			l := New(&local, "info", "service", slog.NewJSONHandler(&exported, nil))
			emit(l)
			for _, buf := range []*bytes.Buffer{&local, &exported} {
				require.NotContains(t, buf.String(), "plain-secret")
				require.Contains(t, buf.String(), Redacted)
			}
		})
	}
}
