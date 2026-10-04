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
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

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

func newServer(t *testing.T, maxAttempts int) (http.Handler, *clock) {
	t.Helper()
	svc, c := newService(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.TraceRequests("auth-test"), httpx.LogAccess(logger))
	NewHandler(svc, NewLimiter(startRedis(t), maxAttempts, time.Minute), false).Mount(r)
	r.Group(func(pr chi.Router) {
		pr.Use(RequireSession(svc))
		pr.Post("/api/v1/things", func(w http.ResponseWriter, req *http.Request) {
			httpx.WriteSuccess(w, req, http.StatusCreated, map[string]bool{"ok": true})
		})
		pr.Post("/api/v1/stream-probe", streamProbe)
	})
	return r, c
}

const probeFrameGap = 300 * time.Millisecond

// streamProbe emits three frames with a pause between them so a test can tell incremental delivery from buffering.
func streamProbe(w http.ResponseWriter, r *http.Request) {
	stream, err := httpx.NewEventStream(w, r, time.Hour)
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
	sess := cookie(rec, CookieSession)
	require.NotNil(t, sess)
	require.True(t, sess.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, sess.SameSite)
	require.NotNil(t, cookie(rec, CookieCSRF))
}

func TestMutationWithoutCSRFIsForbidden(t *testing.T) {
	h, _ := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/things", nil)
	req.AddCookie(cookie(rec, CookieSession))
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	require.Equal(t, http.StatusForbidden, out.Code)
	require.Equal(t, "AUTH_CSRF_INVALID", errCode(t, out))

	req.Header.Set(HeaderCSRF, cookie(rec, CookieCSRF).Value)
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, req)
	require.Equal(t, http.StatusCreated, ok.Code)
}

func TestExpiredSessionIsRejectedOverHTTP(t *testing.T) {
	h, c := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	c.t = c.t.Add(2 * time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie(rec, CookieSession))
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
}

func TestMeReturnsUserWithoutSecrets(t *testing.T) {
	h, _ := newServer(t, 10)
	rec := login(t, h, ownerEmail, ownerPassword)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie(rec, CookieSession))
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	require.Equal(t, http.StatusOK, out.Code)
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
	req.Header.Set(HeaderCSRF, login.Data.CSRFToken)
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
