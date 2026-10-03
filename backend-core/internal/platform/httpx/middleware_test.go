package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccessLogPreservesCommittedStatusAfterPanic(t *testing.T) {
	var logs bytes.Buffer
	r := NewRouter(nil, AccessLog(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.Get("/partial", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("after commit")
	})
	response := do(r, http.MethodGet, "/partial", "en")
	require.Equal(t, http.StatusOK, response.Code)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.EqualValues(t, response.Code, entry["status"])
}

func TestStatusRecorderKeepsFirstFinalResponse(t *testing.T) {
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
