package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type authLogBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *authLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *authLogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

func newAuthenticatedStream(t *testing.T, handler http.HandlerFunc) (*http.Response, *authLogBuffer, context.CancelFunc) {
	t.Helper()
	svc, clk := newService(t)
	clk.t = time.Now()
	logs := &authLogBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.TraceRequests("auth-stream-test"), httpx.LogAccess(logger))
	NewHandler(svc, NewLimiter(startRedis(t), 10, time.Minute), false).Mount(router)
	router.Group(func(pr chi.Router) { pr.Use(RequireSession(svc)); pr.Post("/stream", handler) })
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}
	loginResp, err := client.Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader(`{"email":"`+ownerEmail+`","password":"`+ownerPassword+`"}`))
	require.NoError(t, err)
	var envelope struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&envelope))
	require.NoError(t, loginResp.Body.Close())
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/stream", nil)
	require.NoError(t, err)
	req.Header.Set(HeaderCSRF, envelope.Data.CSRFToken)
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	return resp, logs, cancel
}

func TestSessionAndCSRF_DisconnectCancelsHandler(t *testing.T) {
	cancelled := make(chan struct{})
	resp, logs, cancel := newAuthenticatedStream(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := httpx.NewEventStream(w, r, time.Hour)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer stream.Close()
		if err := stream.Send([]byte(`{"n":1}`)); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		<-stream.Done()
		close(cancelled)
	})
	defer func() { require.NoError(t, resp.Body.Close()) }()
	first, err := readFrame(bufio.NewReader(resp.Body))
	require.NoError(t, err)
	require.Equal(t, "data: {\"n\":1}\n", first)
	cancel()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect did not cancel handler")
	}
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"path":"/stream"`) == 1 }, 5*time.Second, time.Millisecond)
}

func TestSessionAndCSRF_CommittedPanicAbortsWithoutJSON(t *testing.T) {
	resp, logs, _ := newAuthenticatedStream(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := httpx.NewEventStream(w, r, time.Hour)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer stream.Close()
		if err := stream.Send([]byte(`{"n":1}`)); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		panic("test stream panic")
	})
	defer func() { require.NoError(t, resp.Body.Close()) }()
	reader := bufio.NewReader(resp.Body)
	first, err := readFrame(reader)
	require.NoError(t, err)
	require.Equal(t, "data: {\"n\":1}\n", first)
	rest, err := io.ReadAll(reader)
	require.Error(t, err)
	require.Empty(t, rest)
	require.NotContains(t, string(rest), "INTERNAL_ERROR")
	require.NotContains(t, string(rest), "test stream panic")
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"msg":"request failed"`) == 1 }, 5*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"path":"/stream"`) == 1 }, 5*time.Second, time.Millisecond)
}
