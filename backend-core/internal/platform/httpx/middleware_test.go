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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestLogAccess_PreservesCommittedStatusAfterPanic(t *testing.T) {
	var logs bytes.Buffer
	r := NewRouter(nil, LogAccess(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.GET("/partial", wrapHTTP(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("after commit")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial", nil)
	// The response is committed, so recovery aborts the connection instead of appending an error body.
	require.PanicsWithValue(t, http.ErrAbortHandler, func() { r.ServeHTTP(rec, req) })
	require.Equal(t, "partial", rec.Body.String())
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.EqualValues(t, http.StatusOK, entry["status"])
}

func TestGinWriter_KeepsFirstCommittedStatus(t *testing.T) {
	r := NewRouter(nil)
	r.GET("/status", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusCreated)
		c.Writer.WriteHeaderNow()
		c.Writer.WriteHeader(http.StatusInternalServerError)
		require.True(t, ResponseCommitted(c.Writer))
	})
	rec := performRequest(r, http.MethodGet, "/status", "en")
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestRouter_LogsShareRequestID(t *testing.T) {
	for _, requestID := range []string{"req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21", ""} {
		t.Run(requestID, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
			require.NoError(t, err)
			router := NewRouter(NewSlogErrorLogger(logger), LogAccess(logger))
			router.GET("/log", wrapHTTP(func(w http.ResponseWriter, req *http.Request) {
				logger.InfoContext(req.Context(), "handler event")
				WriteError(w, req, errors.ErrInternalError)
			}))
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/log", nil)
			req.Header.Set("X-Request-Id", requestID)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			actualID := response.Header().Get("X-Request-Id")
			if requestID != "" {
				require.Equal(t, requestID, actualID)
			} else {
				require.True(t, strings.HasPrefix(actualID, "req_"))
			}
			lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
			require.Len(t, lines, 3)
			for _, line := range lines {
				var entry map[string]any
				require.NoError(t, json.Unmarshal(line, &entry))
				require.Equal(t, actualID, entry["request_id"])
				require.Equal(t, 1, bytes.Count(line, []byte(`"request_id":`)))
				require.Equal(t, "test", entry["env"])
				require.NotContains(t, entry, "trace_id")
			}
		})
	}
}
