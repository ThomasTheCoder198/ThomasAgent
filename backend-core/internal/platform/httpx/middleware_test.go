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
	"go.opentelemetry.io/otel/trace"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

func TestLogAccess_PreservesCommittedStatusAfterPanic(t *testing.T) {
	var logs bytes.Buffer
	r := NewRouter(nil, LogAccess(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.Get("/partial", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("after commit")
	})
	response := performRequest(r, http.MethodGet, "/partial", "en")
	require.Equal(t, http.StatusOK, response.Code)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.EqualValues(t, response.Code, entry["status"])
}

func TestStatusRecorder_KeepsFirstFinalResponse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*statusRecorder)
		want  int
	}{
		{"explicit", func(s *statusRecorder) {
			s.WriteHeader(http.StatusCreated)
			s.WriteHeader(http.StatusInternalServerError)
		}, http.StatusCreated},
		{"implicit-write", func(s *statusRecorder) { _, _ = s.Write([]byte("body")); s.WriteHeader(http.StatusInternalServerError) }, http.StatusOK},
		{"implicit-flush", func(s *statusRecorder) { s.Flush(); s.WriteHeader(http.StatusInternalServerError) }, http.StatusOK},
		{"informational", func(s *statusRecorder) { s.WriteHeader(http.StatusEarlyHints); s.WriteHeader(http.StatusAccepted) }, http.StatusAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				s := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
				tc.write(s)
				if s.status != tc.want {
					t.Errorf("recorded status = %d, want %d", s.status, tc.want)
				}
			}))
			defer server.Close()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			response, err := server.Client().Do(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, response.Body.Close()) }()
			require.Equal(t, tc.want, response.StatusCode)
		})
	}
}

func TestRouter_LogsShareRequestID(t *testing.T) {
	for _, requestID := range []string{"custom-request", ""} {
		t.Run(requestID, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := logging.NewLogger(&output, "info", "core-test", "test", nil)
			require.NoError(t, err)
			router := NewRouter(NewSlogErrorLogger(logger), LogAccess(logger))
			router.Get("/log", func(w http.ResponseWriter, req *http.Request) {
				logger.InfoContext(req.Context(), "handler event")
				WriteError(w, req, errors.ErrInternalError)
			})
			traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
			require.NoError(t, err)
			spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
			require.NoError(t, err)
			ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
				TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
			}))
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/log", nil)
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
				require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", entry["trace_id"])
			}
		})
	}
}
