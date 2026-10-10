package ragx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	testOrg       = "3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	testRequestID = "req_7d1f2a3b-4c5d-4e6f-8a9b-0c1d2e3f4a5b"
	kbJSON        = `{"id":"8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f","name":"Mặc định","isDefault":true,"embeddingModel":"m","embeddingDims":1536,"fileCount":0,"createdAt":"2026-10-08T00:00:00Z","updatedAt":"2026-10-08T00:00:00Z"}`
)

type seen struct {
	header  http.Header
	claims  map[string]any
	body    []byte
	length  int64
	chunked bool
}

type outboundCall struct{ target, outcome string }

type fakeRecorder struct{ calls []outboundCall }

func (f *fakeRecorder) ObserveOutbound(target, outcome string, _ time.Duration) {
	f.calls = append(f.calls, outboundCall{target, outcome})
}

func testConfig(url string) config.RagxConfig {
	return config.RagxConfig{
		URL: url, ConnectTimeout: time.Second, RequestTimeout: 200 * time.Millisecond, ResponseHeaderTimeout: time.Second,
		TransferTimeout: time.Minute, UploadMaxBytes: 1 << 30, MaxErrorBodyBytes: 1 << 10,
	}
}

func newCaller(t *testing.T, handler http.HandlerFunc) (*Caller, *[]seen, *fakeRecorder, *bytes.Buffer) {
	t.Helper()
	var requests []seen
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		parts := strings.Split(token, ".")
		var claims map[string]any
		if len(parts) == 3 {
			raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
			_ = json.Unmarshal(raw, &claims)
		}
		requests = append(requests, seen{header: r.Header.Clone(), claims: claims, body: body, length: r.ContentLength, chunked: len(r.TransferEncoding) > 0})
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	signer := apptoken.NewAppTokenSigner(key, config.AppConfig{SigningKeyID: "dev-1", TokenIssuer: "thomas-core", TokenAudience: "ragx", TokenTTL: time.Minute},
		time.Now, func() string { return "jti-test" })
	recorder := &fakeRecorder{}
	var logs bytes.Buffer
	log, err := logging.NewLogger(&logs, "debug", "test", "test", nil)
	require.NoError(t, err)
	caller, err := NewRagxCaller(testConfig(server.URL), signer, recorder, log)
	require.NoError(t, err)
	return caller, &requests, recorder, &logs
}

func orgContext() context.Context {
	return requestid.NewContext(tenant.WithID(context.Background(), testOrg), testRequestID)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Ragx-Trace-Id", "0af7651916cd43dd8448eb211c80319c")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestCaller_SignsOidFromTenantAndRidEqualToTheSentRequestID(t *testing.T) {
	caller, requests, recorder, logs := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":[`+kbJSON+`],"meta":{"requestId":"`+testRequestID+`"}}`)
	})
	kbs, err := caller.ListKnowledgeBases(orgContext())
	require.NoError(t, err)
	require.Len(t, kbs, 1)
	got := (*requests)[0]
	require.Equal(t, testRequestID, got.header.Get("X-Request-Id"))
	require.Equal(t, testRequestID, got.claims["rid"])
	require.Equal(t, testOrg, got.claims["oid"])
	require.Equal(t, "kb:read", got.claims["scp"])
	require.Equal(t, []outboundCall{{"ragx", "success"}}, recorder.calls)
	require.Contains(t, logs.String(), `"msg":"ragx call"`)
	require.Contains(t, logs.String(), `"ragx_trace_id":"0af7651916cd43dd8448eb211c80319c"`)
	require.NotContains(t, logs.String(), "Bearer")
}

func TestCaller_UsesOneFreshJobIDWhenTheContextHasNone(t *testing.T) {
	caller, requests, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":[],"meta":{"requestId":"x"}}`)
	})
	_, err := caller.ListKnowledgeBases(tenant.WithID(context.Background(), testOrg))
	require.NoError(t, err)
	header := (*requests)[0].header.Get("X-Request-Id")
	require.True(t, strings.HasPrefix(header, requestid.JobPrefix), header)
	require.Equal(t, header, (*requests)[0].claims["rid"])
}

func TestCaller_RequiresTrustedTenant(t *testing.T) {
	caller, requests, _, _ := newCaller(t, func(http.ResponseWriter, *http.Request) {})
	_, err := caller.ListKnowledgeBases(context.Background())
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	require.Empty(t, *requests)
}

func TestCaller_PassesCatalogClientErrorsThrough(t *testing.T) {
	caller, _, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"error":{"code":"KB_NOT_FOUND","message":"x","details":{"kbId":"k"}}}`)
	})
	_, err := caller.GetKnowledgeBase(orgContext(), uuid.New())
	require.ErrorIs(t, err, errors.ErrKbNotFound)
	require.Equal(t, map[string]any{"kbId": "k"}, errors.ToAppError(err).Details)
}

func TestCaller_MapsFailuresAndCredentialRejectionsToRagxUnavailable(t *testing.T) {
	for name, respond := range map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 500, `{"error":{"code":"INTERNAL_ERROR","message":"x"}}`)
		},
		"503": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
		"401 app token": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 401, `{"error":{"code":"APP_TOKEN_INVALID","message":"x"}}`)
		},
		"403 scope": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 403, `{"error":{"code":"FORBIDDEN","message":"x"}}`)
		},
		"unknown code": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 404, `{"error":{"code":"NOT_A_CODE","message":"x"}}`)
		},
		"status drift": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 400, `{"error":{"code":"KB_NOT_FOUND","message":"x"}}`)
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
		},
		"huge body": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 404, strings.Repeat(" ", 4096)+`{"error":{"code":"KB_NOT_FOUND"}}`)
		},
		"timeout": func(w http.ResponseWriter, _ *http.Request) { time.Sleep(time.Second); w.WriteHeader(200) },
	} {
		t.Run(name, func(t *testing.T) {
			caller, _, _, _ := newCaller(t, respond)
			_, err := caller.ListKnowledgeBases(orgContext())
			require.ErrorIs(t, err, errors.ErrRagxUnavailable)
		})
	}
}

func TestCaller_UnreachableRagxIsUnavailable(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	signer := apptoken.NewAppTokenSigner(key, config.AppConfig{SigningKeyID: "k", TokenIssuer: "i", TokenAudience: "a", TokenTTL: time.Minute}, time.Now, uuid.NewString)
	caller, err := NewRagxCaller(testConfig("http://127.0.0.1:1"), signer, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	_, err = caller.ListKnowledgeBases(orgContext())
	require.ErrorIs(t, err, errors.ErrRagxUnavailable)
}

func TestCaller_UploadStreamsWithContentLengthAndIdempotencyKey(t *testing.T) {
	payload := bytes.Repeat([]byte("thomas"), 200_000)
	caller, requests, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, `{"data":{"id":"`+uuid.NewString()+`","kbId":"8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f","name":"a.pdf","version":1,"contentType":"application/pdf","sizeBytes":1,"sha256":"x","status":"pending","errorCode":"","degraded":false,"createdAt":"2026-10-08T00:00:00Z","updatedAt":"2026-10-08T00:00:00Z"},"meta":{"requestId":"x"}}`)
	})
	_, created, err := caller.UploadFile(orgContext(), Upload{
		KnowledgeBaseID: uuid.New(), ContentType: "multipart/form-data; boundary=x", ContentLength: int64(len(payload)),
		Body: struct{ io.Reader }{bytes.NewReader(payload)}, IdempotencyKey: "upload-key-0001",
	})
	require.NoError(t, err)
	require.True(t, created)
	got := (*requests)[0]
	require.Equal(t, int64(len(payload)), got.length)
	require.False(t, got.chunked, "ragx requires Content-Length, never chunked")
	require.Equal(t, "upload-key-0001", got.header.Get("Idempotency-Key"))
	require.Equal(t, "kb:write", got.claims["scp"])
	want, have := sha256.Sum256(payload), sha256.Sum256(got.body)
	require.Equal(t, hex.EncodeToString(want[:]), hex.EncodeToString(have[:]))
}
