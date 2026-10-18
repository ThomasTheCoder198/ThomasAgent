package integration

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

const (
	appTokenVectorPath = "../../../contracts/app-token.json"
	appTokenType       = "thomas-app+jwt"
	provisionPathStart = "/v1/orgs/"
)

// fixedDefaultKB is the default Knowledge Base id the fake ragx answers to a provision call.
var fixedDefaultKB = uuid.MustParse("8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f")

type appTokenVector struct {
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
	Keys     []struct {
		KID            string `json:"kid"`
		PublicKey      string `json:"publicKey"`
		PrivateKeySeed string `json:"privateKeySeed"`
	} `json:"keys"`
}

// fakeRagx is an httptest ragx. It verifies every App token with the shared test vector's key
// (contracts/app-token.json, kid test-1) the way ragx does (EdDSA, typ, iss, aud, rid == X-Request-Id). A token it
// cannot verify gets 401 APP_TOKEN_INVALID, like ragx.
type fakeRagx struct {
	server  *httptest.Server
	vector  appTokenVector
	mu      sync.Mutex
	seen    []ragxCall
	respond http.HandlerFunc
}

type ragxCall struct {
	method, path, rawQuery string
	header                 http.Header
	claims                 jwt.MapClaims
	body                   []byte
}

func newFakeRagx(t *testing.T) *fakeRagx {
	t.Helper()
	raw, err := os.ReadFile(appTokenVectorPath)
	require.NoError(t, err)
	fake := &fakeRagx{respond: provisionOK}
	require.NoError(t, json.Unmarshal(raw, &fake.vector))
	require.NotEmpty(t, fake.vector.Keys)
	publicKey, err := base64.StdEncoding.DecodeString(fake.vector.Keys[0].PublicKey)
	require.NoError(t, err)
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body)) // the responder may read it again
		claims, ok := fake.verify(r, ed25519.PublicKey(publicKey))
		if !ok {
			ragxError(http.StatusUnauthorized, "APP_TOKEN_INVALID")(w, r)
			return
		}
		fake.mu.Lock()
		fake.seen = append(fake.seen, ragxCall{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, header: r.Header.Clone(), claims: claims, body: body})
		respond := fake.respond
		fake.mu.Unlock()
		respond(w, r)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// verify checks the token the way ragx does; it never trusts the claims before the signature is valid.
func (f *fakeRagx) verify(r *http.Request, key ed25519.PublicKey) (jwt.MapClaims, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(f.vector.Issuer), jwt.WithAudience(f.vector.Audience))
	if err != nil || !parsed.Valid {
		return nil, false
	}
	return claims, parsed.Header["kid"] == f.vector.Keys[0].KID && parsed.Header["typ"] == appTokenType &&
		claims["rid"] == r.Header.Get("X-Request-Id")
}

func (f *fakeRagx) setResponder(handler http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.respond = handler
}

func (f *fakeRagx) calls() []ragxCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ragxCall(nil), f.seen...)
}

func (f *fakeRagx) caller(t *testing.T, logs io.Writer) *ragx.Caller {
	t.Helper()
	seed, err := base64.StdEncoding.DecodeString(f.vector.Keys[0].PrivateKeySeed)
	require.NoError(t, err)
	signer := apptoken.NewAppTokenSigner(ed25519.NewKeyFromSeed(seed), config.AppConfig{
		SigningKeyID: f.vector.Keys[0].KID, TokenIssuer: f.vector.Issuer, TokenAudience: f.vector.Audience, TokenTTL: time.Minute,
	}, time.Now, uuid.NewString)
	caller, err := ragx.NewRagxCaller(config.RagxConfig{
		URL: f.server.URL, ConnectTimeout: time.Second, RequestTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		MaxErrorBodyBytes: 1 << 16, MaxResponseBytes: 1 << 16, ProvisionAttempts: 1, ToolTimeout: 5 * time.Second,
	}, signer, nil, logger(t, logs), calllog.New(logger(t, logs), nil, config.LLMLogConfig{ContentMaxBytes: 1}))
	require.NoError(t, err)
	return caller
}

// logger writes JSON logs to w, like production.
func logger(t *testing.T, w io.Writer) *slog.Logger {
	t.Helper()
	l, err := logging.NewLogger(w, "debug", "test", "test", nil)
	require.NoError(t, err)
	return l
}

// provisionOK answers PUT /v1/orgs/{orgId} like ragx, with fixedDefaultKB.
func provisionOK(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		ExternalParsing bool   `json:"externalParsing"`
		SettingsVersion int64  `json:"settingsVersion"`
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &body)
	writeJSONBody(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"orgId": strings.TrimPrefix(r.URL.Path, provisionPathStart), "name": body.Name, "externalParsing": body.ExternalParsing,
			"settingsVersion": body.SettingsVersion, "defaultKbId": fixedDefaultKB.String(), "createdAt": time.Now().UTC().Format(time.RFC3339),
		},
		"meta": map[string]any{"requestId": r.Header.Get("X-Request-Id")},
	})
}

// ragxError writes the shared error format with the request id ragx would echo.
func ragxError(status int, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, status, map[string]any{
			"error": map[string]any{"code": code, "message": "", "requestId": r.Header.Get("X-Request-Id")},
		})
	}
}

// ragxJSON answers every call with a JSON body.
func ragxJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// ragxStatus answers every call with a status and no body.
func ragxStatus(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }
}

// slowly waits before answering, and gives up as soon as the caller does, so a timed-out test leaves nothing running.
func slowly(delay time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			next(w, r)
		case <-r.Context().Done():
		}
	}
}

func writeJSONBody(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
