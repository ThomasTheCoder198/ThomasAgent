package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	c, err := tcredis.Run(context.Background(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Terminate(context.Background())) })
	url, err := c.ConnectionString(context.Background())
	require.NoError(t, err)
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func mountAuthForTest(r *gin.Engine, p ControllerParams) {
	ctrl := NewController(p)
	r.POST("/api/v1/auth/login", ctrl.HandleLogin)
	protected := r.Group("", RequireSession(p.Service.(sessionAuthenticator)))
	protected.POST("/api/v1/auth/logout", ctrl.HandleLogout)
	protected.GET("/api/v1/auth/me", ctrl.HandleMe)
}

func wrapAuthHTTP(h http.HandlerFunc) gin.HandlerFunc { return gin.WrapF(h) }

func newServer(t *testing.T, maxAttempts int) (http.Handler, *clock) {
	t.Helper()
	svc, c := newTestService(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	mountAuthForTest(r, ControllerParams{Service: svc, Limiter: newTestLimiter(startRedis(t), maxAttempts)})
	pr := r.Group("", RequireSession(svc))
	pr.POST("/api/v1/things", wrapAuthHTTP(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteSuccess(w, req, http.StatusCreated, map[string]bool{"ok": true})
	}))
	pr.POST("/api/v1/stream-probe", wrapAuthHTTP(streamProbe))
	return r, c
}

const probeFrameGap = 300 * time.Millisecond

// streamProbe emits three frames with a pause between them so a test can tell incremental delivery from buffering.

func streamProbe(w http.ResponseWriter, r *http.Request) {
	stream, err := httpx.NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
	if err != nil {
		return
	}
	defer stream.Close()
	if err := stream.Send([]byte(`{"n":1}`)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	select {
	case <-time.After(probeFrameGap):
	case <-stream.Done():
		return
	}
	if err := stream.Send([]byte(`{"n":2}`)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	select {
	case <-time.After(probeFrameGap):
	case <-stream.Done():
		return
	}
	if err := stream.Send([]byte(`{"n":3}`)); err != nil {
		httpx.WriteError(w, r, err)
	}
}

func login(t *testing.T, h http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct{ Error struct{ Code string } }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Error.Code
}

func TestLoginSetsCookies(t *testing.T) {
	h, _ := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	require.Equal(t, http.StatusOK, rec.Code)
	sess := cookie(rec, SessionCookie)
	require.NotNil(t, sess)
	require.True(t, sess.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, sess.SameSite)
	require.NotNil(t, cookie(rec, CSRFCookie))
}

func TestExpiredSessionIsRejectedOverHTTP(t *testing.T) {
	service, _ := newTestService(t)
	h := httpx.NewRouter(nil)
	mountAuthForTest(h, ControllerParams{Service: service, Limiter: newTestLimiter(startRedis(t), 10)})
	rec := login(t, h, ownerEmail, ownerPassword)
	_, err := service.db.Exec(t.Context(), `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hashSessionToken(cookie(rec, SessionCookie).Value))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie(rec, SessionCookie))
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	require.Equal(t, http.StatusUnauthorized, out.Code)
	require.Equal(t, "AUTH_SESSION_EXPIRED", errCode(t, out))
}

func TestLoginIsRateLimited(t *testing.T) {
	h, _ := newServer(t, 2)
	for i := 0; i < 2; i++ {
		require.Equal(t, http.StatusUnauthorized, login(t, h, ownerEmail, "wrong password!!").Code)
	}
	rec := login(t, h, ownerEmail, ownerPassword)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "RATE_LIMITED", errCode(t, rec))
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestLoginSuccessResetsFailedAttemptBudget(t *testing.T) {
	h, _ := newServer(t, 2)
	for range 2 {
		require.Equal(t, http.StatusUnauthorized, login(t, h, ownerEmail, "incorrect-password").Code)
		rec := login(t, h, ownerEmail, ownerPassword)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestLoginLimiterSeparatesForwardedIPsAndEmails(t *testing.T) {
	svc, _ := newTestService(t)
	client := startRedis(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger))
	mountAuthForTest(router, ControllerParams{Service: svc, Limiter: NewLoginLimiter(client, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 10, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("test-limiter-hmac-key-at-least-32-chars")), TrustedProxyHeader: "X-Trusted-Client-IP", TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, Logger: logger})
	attempt := func(email, ip string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"wrong-password"}`))
		request.RemoteAddr = "10.0.0.1:1234"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Trusted-Client-IP", ip)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	require.Equal(t, 401, attempt(ownerEmail, "192.0.2.1"))
	require.Equal(t, 429, attempt(ownerEmail, "192.0.2.1"))
	require.Equal(t, 401, attempt(ownerEmail, "192.0.2.2"))
	require.Equal(t, 401, attempt("different@example.com", "192.0.2.1"))
	keys, err := client.Keys(t.Context(), loginLimiterKeyPrefix+"*").Result()
	require.NoError(t, err)
	for _, key := range keys {
		require.NotContains(t, key, "@")
	}
}

func TestMeReturnsUserWithoutSecrets(t *testing.T) {
	h, _ := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie(rec, SessionCookie))
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	require.Equal(t, http.StatusOK, out.Code)
	require.Equal(t, "no-store", out.Header().Get("Cache-Control"))
	require.Contains(t, out.Body.String(), ownerEmail)
	require.NotContains(t, out.Body.String(), "password")
}

func readFrame(reader *bufio.Reader) (string, error) {
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return frame.String(), err
		}
		if line == "\n" {
			return frame.String(), nil
		}
		frame.WriteString(line)
	}
}

// Review Focus: session and CSRF middleware must not break flushing. The first frame has to reach the client before
// the handler's pause ends, over real HTTP with real cookies and the real CSRF check.
