package auth

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestLogin_ThresholdAuditBoundedAndWarningsRedacted(t *testing.T) {
	svc, _ := newService(t)
	client := startRedis(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	lim := NewLoginLimiter(client, config.AuthConfig{LoginMaxAttempts: 10, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("test-hmac-key-with-at-least-32-bytes"))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger))
	NewHandlerWithProxyHeader(svc, lim, false, "", logger).Mount(router)
	for index, want := range []int{401, 401, 429, 429, 429} {
		require.Equal(t, want, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "").Code, "attempt %d", index)
	}
	var count int
	require.NoError(t, svc.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE action='auth.login.failed'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NotContains(t, logs.String(), ownerEmail)
	require.NotContains(t, logs.String(), "wrong-password")
	require.NotContains(t, logs.String(), "test-hmac-key")
	decoder := json.NewDecoder(&logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		require.Contains(t, record, "trace_id")
	}
	require.NoError(t, client.FlushDB(t.Context()).Err())
	svc.db = failingExecDB{DBTX: svc.db, statement: "INSERT INTO audit_events", failure: stderrors.New("audit unavailable")}
	for _, want := range []int{401, 401, 429, 429} {
		require.Equal(t, want, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "").Code)
	}
}

func TestLogin_LimiterAndArgonSpansFollowRequest(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	svc, _ := newService(t)
	client := startRedis(t)
	lim := NewLimiter(client, 2, time.Minute)
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.TraceRequests("core.auth"))
	NewHandlerWithProxyHeader(svc, lim, false, "", logger).Mount(router)
	require.Equal(t, 401, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "").Code)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "test-parent")
	req := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	require.NoError(t, lim.Reset(req.Context(), "unused"))
	parent.End()
	found := map[string]bool{}
	for _, span := range recorder.Ended() {
		switch span.Name() {
		case "core.auth.limiter.check", "core.auth.argon.compare", "core.auth.limiter.reset":
			require.True(t, span.Parent().IsValid())
			found[span.Name()] = true
		}
		for _, attr := range span.Attributes() {
			require.NotContains(t, attr.Value.AsString(), ownerEmail)
			require.NotContains(t, attr.Value.AsString(), "wrong-password")
		}
	}
	require.True(t, found["core.auth.limiter.check"])
	require.True(t, found["core.auth.argon.compare"])
	require.True(t, found["core.auth.limiter.reset"])
}
