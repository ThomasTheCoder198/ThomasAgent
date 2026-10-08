package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

func serveWithAccessLog(t *testing.T, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var logs bytes.Buffer
	r := NewRouter(nil, LogAccess(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	r.GET("/api/v1/providers/:id", wrapHTTP(func(w http.ResponseWriter, req *http.Request) {
		WriteSuccess(w, req, http.StatusOK, map[string]string{"id": "x"})
	}))
	r.GET("/conflict", wrapHTTP(func(w http.ResponseWriter, req *http.Request) { WriteError(w, req, errors.ErrConflict) }))
	r.GET("/boom", wrapHTTP(func(w http.ResponseWriter, req *http.Request) { WriteError(w, req, errors.ErrInternalError) }))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	return rec, entry
}

func newRequest(path string) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
}

func TestAssignRequestID_RejectsForgedHeaderWithoutLoggingIt(t *testing.T) {
	forged := "req_abcdefgh\"} injected"
	req := newRequest("/api/v1/providers/42")
	req.Header.Set(requestid.Header, forged)
	rec, entry := serveWithAccessLog(t, req)
	got := rec.Header().Get(requestid.Header)
	require.True(t, requestid.IsValid(got))
	require.True(t, strings.HasPrefix(got, requestid.RequestPrefix))
	require.NotEqual(t, forged, got)
	require.Equal(t, "generated", entry["request_id_source"])
	require.Equal(t, true, entry["client_request_id_rejected"])
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "injected")
}

func TestAssignRequestID_AcceptsJobPrefix(t *testing.T) {
	req := newRequest("/api/v1/providers/42")
	req.Header.Set(requestid.Header, "job_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
	rec, entry := serveWithAccessLog(t, req)
	require.Equal(t, "job_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21", rec.Header().Get(requestid.Header))
	require.NotContains(t, entry, "request_id_source")
	require.NotContains(t, entry, "client_request_id_rejected")
}

func TestAssignRequestID_MissingHeaderIsGeneratedNotRejected(t *testing.T) {
	_, entry := serveWithAccessLog(t, newRequest("/api/v1/providers/42"))
	require.Equal(t, "generated", entry["request_id_source"])
	require.NotContains(t, entry, "client_request_id_rejected")
}

func TestLogAccess_UsesRoutePatternNotRawPath(t *testing.T) {
	_, entry := serveWithAccessLog(t, newRequest("/api/v1/providers/secret-looking-id"))
	require.Equal(t, "/api/v1/providers/{id}", entry["route"])
	require.NotContains(t, entry, "path")
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-looking-id")
	require.Contains(t, entry, "duration_ms")
}

func TestLogAccess_UnknownRouteIsUnmatched(t *testing.T) {
	_, entry := serveWithAccessLog(t, newRequest("/does/not/exist"))
	require.Equal(t, "unmatched", entry["route"])
	require.EqualValues(t, http.StatusNotFound, entry["status"])
}

func TestLogAccess_LevelFollowsStatusClass(t *testing.T) {
	for _, tc := range []struct{ path, level string }{
		{"/api/v1/providers/1", "INFO"},
		{"/conflict", "WARN"},
		{"/boom", "ERROR"},
	} {
		_, entry := serveWithAccessLog(t, newRequest(tc.path))
		require.Equal(t, tc.level, entry["level"], tc.path)
	}
}

func TestLogAccess_RecordsOnlyWellFormedIdempotencyKeys(t *testing.T) {
	good := newRequest("/api/v1/providers/1")
	good.Header.Set("Idempotency-Key", "3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
	_, entry := serveWithAccessLog(t, good)
	require.Equal(t, "3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21", entry["idempotency_key"])

	bad := newRequest("/api/v1/providers/1")
	bad.Header.Set("Idempotency-Key", "short")
	_, entry = serveWithAccessLog(t, bad)
	require.NotContains(t, entry, "idempotency_key")
	require.Equal(t, true, entry["idempotency_key_rejected"])
}
