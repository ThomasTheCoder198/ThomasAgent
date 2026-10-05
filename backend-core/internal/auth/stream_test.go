package auth

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
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
		stream, err := httpx.NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
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

func TestSessionAndCSRF_NonReadingClientBoundsShutdown(t *testing.T) {
	svc, _ := newService(t)
	_, session, err := svc.Login(tenant.WithID(t.Context(), tenant.PlatformID), ownerEmail, ownerPassword)
	require.NoError(t, err)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.TraceRequests("blocked-stream"), httpx.LogAccess(logger))
	result := make(chan error, 1)
	router.Group(func(pr chi.Router) {
		pr.Use(RequireSession(svc))
		pr.Post("/stream", func(w http.ResponseWriter, r *http.Request) {
			stream, err := httpx.NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: 50 * time.Millisecond})
			if err != nil {
				result <- err
				return
			}
			err = stream.Send(bytes.Repeat([]byte{'x'}, 1<<20))
			stream.Close()
			result <- err
		})
	})
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	request := httptest.NewRequest(http.MethodPost, "/stream", nil)
	request.AddCookie(&http.Cookie{Name: CookieSession, Value: session.Token})
	request.Header.Set(HeaderCSRF, session.CSRFToken)
	go router.ServeHTTP(&blockedStreamWriter{conn: server, header: make(http.Header)}, request)
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after its write deadline through tracing, session and CSRF middleware")
	}
}

type blockedStreamWriter struct {
	conn   net.Conn
	header http.Header
}

func (w *blockedStreamWriter) Header() http.Header         { return w.header }
func (w *blockedStreamWriter) WriteHeader(int)             {}
func (w *blockedStreamWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }
func (w *blockedStreamWriter) Flush()                      {}
func (w *blockedStreamWriter) SetWriteDeadline(deadline time.Time) error {
	return w.conn.SetWriteDeadline(deadline)
}

func TestSessionAndCSRF_CommittedPanicAbortsWithoutJSON(t *testing.T) {
	resp, logs, _ := newAuthenticatedStream(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := httpx.NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
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
