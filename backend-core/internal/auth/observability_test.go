package auth

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"testing"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

func TestLogin_ThresholdAuditBoundedAndWarningsRedacted(t *testing.T) {
	svc, _ := newService(t)
	client := startRedis(t)
	var logs bytes.Buffer
	logger, err := logging.NewLogger(&logs, "info", "auth-test", "test", nil)
	require.NoError(t, err)
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
		require.Contains(t, record, "request_id")
	}
	require.NoError(t, client.FlushDB(t.Context()).Err())
	svc.db = failingExecDB{DBTX: svc.db, statement: "INSERT INTO audit_events", failure: stderrors.New("audit unavailable")}
	for _, want := range []int{401, 401, 429, 429} {
		require.Equal(t, want, attemptFrom(router, ownerEmail, "192.0.2.1:9999", "").Code)
	}
}
