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

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
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
	svc, clk := newTestService(t)
	clk.t = time.Now()
	logs := &authLogBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	mountAuthForTest(router, ControllerParams{Service: svc, Limiter: NewLimiter(startRedis(t), 10, time.Minute)})
	router.Group("", RequireSession(svc)).POST("/stream", wrapAuthHTTP(handler))
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
	req.Header.Set(CSRFHeader, envelope.Data.CSRFToken)
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
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"route":"/stream"`) == 1 }, 5*time.Second, time.Millisecond)
}

func TestSessionAndCSRF_NonReadingClientBoundsShutdown(t *testing.T) {
	svc, _ := newTestService(t)
	_, session, err := svc.Login(tenant.WithID(t.Context(), tenant.PlatformID), ownerEmail, ownerPassword)
	require.NoError(t, err)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	result := make(chan error, 1)
	pr := router.Group("", RequireSession(svc))
	pr.POST("/stream", wrapAuthHTTP(func(w http.ResponseWriter, r *http.Request) {
		stream, err := httpx.NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: 50 * time.Millisecond})
		if err != nil {
			result <- err
			return
		}
		err = stream.Send(bytes.Repeat([]byte{'x'}, 1<<20))
		stream.Close()
		result <- err
	}))
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	request := httptest.NewRequest(http.MethodPost, "/stream", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: session.Token})
	request.Header.Set(CSRFHeader, session.CSRFToken)
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

func (w *blockedStreamWriter) Header() http.Header { return w.header }

func (w *blockedStreamWriter) WriteHeader(int) {}

func (w *blockedStreamWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }

func (w *blockedStreamWriter) Flush() {}

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
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"route":"/stream"`) == 1 }, 5*time.Second, time.Millisecond)
}

func TestRequireSessionDerivesTenantFromSessionRow(t *testing.T) {
	svc, _ := newTestService(t)
	platformCtx := tenant.WithID(t.Context(), tenant.PlatformID)
	otherCtx := tenant.WithID(t.Context(), "other")
	other, err := svc.repository.CreateUser(otherCtx, "other@example.com", "hash")
	require.NoError(t, err)
	token := "other-session-token"
	require.NoError(t, svc.repository.CreateSession(otherCtx, other.ID, hashSessionToken(token), "csrf", time.Now().Add(time.Hour)))
	handler := httpx.NewRouter(nil)
	handler.GET("/tenant-data", RequireSession(svc), wrapAuthHTTP(func(w http.ResponseWriter, r *http.Request) {
		scope, err := tenant.ID(r.Context())
		require.NoError(t, err)
		require.Equal(t, "other", scope)
		_, _, err = svc.repository.FindUserByEmail(r.Context(), ownerEmail)
		require.ErrorIs(t, err, errors.ErrNotFound, "a tenant A session must not read tenant B's user")
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/tenant-data", nil).WithContext(platformCtx)
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestMutationWithoutCSRFIsForbidden(t *testing.T) {
	h, _ := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/things", nil)
	req.AddCookie(cookie(rec, SessionCookie))
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	require.Equal(t, http.StatusForbidden, out.Code)
	require.Equal(t, "AUTH_CSRF_INVALID", errCode(t, out))

	req.Header.Set(CSRFHeader, cookie(rec, CSRFCookie).Value)
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, req)
	require.Equal(t, http.StatusCreated, ok.Code)
}

func TestSessionAndCSRF_StreamIncrementally(t *testing.T) {
	h, clk := newServer(t, 10)
	// Cookies carry an absolute expiry. The fixed test clock lies in the past, so a cookie jar would drop them.
	clk.t = time.Now()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}

	loginBody := `{"email":"` + ownerEmail + `","password":"` + ownerPassword + `"}`
	loginResp, err := client.Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader(loginBody))
	require.NoError(t, err)
	defer func() { require.NoError(t, loginResp.Body.Close()) }()
	var login struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&login))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/stream-probe", nil)
	require.NoError(t, err)
	req.Header.Set(CSRFHeader, login.Data.CSRFToken)
	started := time.Now()
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	reader := bufio.NewReader(resp.Body)
	first, err := readFrame(reader)
	require.NoError(t, err)
	firstAt := time.Since(started)
	second, err := readFrame(reader)
	require.NoError(t, err)
	secondAt := time.Since(started)
	third, err := readFrame(reader)
	require.NoError(t, err)
	thirdAt := time.Since(started)
	require.Equal(t, "data: {\"n\":3}\n", third)
	require.GreaterOrEqual(t, thirdAt-secondAt, probeFrameGap/2)

	require.Equal(t, "data: {\"n\":1}\n", first)
	require.Equal(t, "data: {\"n\":2}\n", second)
	require.Less(t, firstAt, probeFrameGap, "the first frame must arrive before the handler's pause ends")
	require.GreaterOrEqual(t, secondAt-firstAt, probeFrameGap/2)
}
