package ragx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const testRoute = "/api/v1/orgs/:orgId/apps/ragx/*path"

func readRequest() ForwardRequest {
	return ForwardRequest{Method: http.MethodGet, Path: "knowledge-bases", RawQuery: "limit=5", Route: testRoute, Scope: apptoken.ScopeKBRead}
}

func reply(status int, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// replay runs a handler locally, so a test compares Forward's result with what ragx really sent.
func replay(handler http.HandlerFunc) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	return recorder
}

func TestForward_ErrorTable(t *testing.T) {
	const jsonType = "application/json"
	for _, tc := range []struct {
		name    string
		respond http.HandlerFunc
		want    error // nil: the response passes through verbatim
	}{
		{"200 passes", reply(200, jsonType, `{"data":[],"meta":{"requestId":"x"}}`), nil},
		{"201 passes", reply(201, jsonType, `{"data":{},"meta":{"requestId":"x"}}`), nil},
		{"204 passes", reply(204, "", ""), nil},
		{"catalog 404 passes", reply(404, jsonType, `{"error":{"code":"KB_NOT_FOUND","message":"x","details":{"kbId":"k"}}}`), nil},
		{"catalog 409 passes", reply(409, jsonType, `{"error":{"code":"CONFLICT","message":"x"}}`), nil},
		{"catalog 400 passes", reply(400, jsonType, `{"error":{"code":"VALIDATION_FAILED","message":"x","details":{"fields":[]}}}`), nil},
		{"catalog 413 passes", reply(413, jsonType, `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"x"}}`), nil},
		{"401 app token", reply(401, jsonType, `{"error":{"code":"APP_TOKEN_INVALID","message":"x"}}`), errors.ErrRagxUnavailable},
		{"403 scope", reply(403, jsonType, `{"error":{"code":"FORBIDDEN","message":"x"}}`), errors.ErrNotFound},
		{"500", reply(500, jsonType, `{"error":{"code":"INTERNAL_ERROR","message":"x"}}`), errors.ErrRagxUnavailable},
		{"503 without body", reply(503, "", ""), errors.ErrRagxUnavailable},
		{"302 redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
		}, errors.ErrRagxUnavailable},
		{"unknown code", reply(404, jsonType, `{"error":{"code":"NOT_A_CODE","message":"x"}}`), errors.ErrRagxUnavailable},
		{"code and status drift", reply(400, jsonType, `{"error":{"code":"KB_NOT_FOUND","message":"x"}}`), errors.ErrRagxUnavailable},
		{"4xx that is not JSON", reply(404, "text/plain", "nope"), errors.ErrRagxUnavailable},
		{"body over the cap", reply(200, jsonType, `{"data":"`+strings.Repeat("x", 2048)+`"}`), errors.ErrRagxUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, _, _, _ := newCaller(t, tc.respond)
			res, err := caller.Forward(orgContext(), readRequest())
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				require.Equal(t, ForwardResponse{}, res)
				return
			}
			require.NoError(t, err)
			sent := replay(tc.respond)
			require.Equal(t, sent.Code, res.Status)
			require.Equal(t, sent.Header().Get("Content-Type"), res.ContentType)
			require.Equal(t, sent.Body.Bytes(), res.Body)
		})
	}
}

func TestForward_SendsOnlyWhatTheRequestCarries(t *testing.T) {
	caller, requests, _, _ := newCaller(t, reply(200, "application/json", `{"data":{},"meta":{"requestId":"x"}}`))
	body := []byte(`{"name":"Chính sách"}`)
	_, err := caller.Forward(orgContext(), ForwardRequest{
		Method: http.MethodPatch, Path: "knowledge-bases/8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f", RawQuery: "a=1&b=2", Route: testRoute,
		ContentType: "application/json", IdempotencyKey: "idem-key-0001", AcceptLanguage: "en", Body: body, Scope: apptoken.ScopeKBWrite,
	})
	require.NoError(t, err)
	got := (*requests)[0]
	require.Equal(t, http.MethodPatch, got.method)
	require.Equal(t, "/v1/knowledge-bases/8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f", got.path)
	require.Equal(t, "a=1&b=2", got.rawQuery)
	require.Equal(t, body, got.body)
	require.EqualValues(t, len(body), got.length)
	require.Equal(t, "kb:write", got.claims["scp"])
	require.Equal(t, testOrg, got.claims["oid"])
	require.Equal(t, testRequestID, got.claims["rid"])
	require.NotContains(t, got.claims, "kb", "the proxy token never carries a KB")
	for name, want := range map[string]string{
		"Content-Type": "application/json", "Idempotency-Key": "idem-key-0001", "Accept-Language": "en", "X-Request-Id": testRequestID,
	} {
		require.Equal(t, want, got.header.Get(name), name)
	}
	require.Empty(t, got.header.Get("Cookie"))
	require.True(t, strings.HasPrefix(got.header.Get("Authorization"), "Bearer "))
}

func TestForward_UsesTheRouteDeadlineNotTheTypedCallTimeout(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		reply(200, "application/json", `{"data":[]}`)(w, r)
	}
	// testConfig's RequestTimeout is 200ms: Forward must not apply it.
	caller, _, _, _ := newCaller(t, slow)
	_, err := caller.Forward(orgContext(), readRequest())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(orgContext(), 50*time.Millisecond)
	defer cancel()
	_, err = caller.Forward(ctx, readRequest())
	require.ErrorIs(t, err, errors.ErrRagxUnavailable)
}

func TestForward_RequiresAnOrgTenantBeforeAnyRequest(t *testing.T) {
	caller, requests, _, _ := newCaller(t, reply(200, "application/json", `{}`))
	_, err := caller.Forward(context.Background(), readRequest())
	require.ErrorIs(t, err, errors.ErrMissingTenant)
	_, err = caller.Forward(tenant.WithID(context.Background(), tenant.PlatformID), readRequest())
	require.ErrorIs(t, err, errors.ErrInternalError, "the platform tenant can never be signed")
	require.Empty(t, *requests)
}

func TestForward_LogsOneLineWithMethodRouteAndJTI(t *testing.T) {
	caller, requests, recorder, logs := newCaller(t, reply(200, "application/json", `{"data":"secret-body-marker"}`))
	_, err := caller.Forward(orgContext(), readRequest())
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(logs.String(), `"msg":"ragx call"`))
	// The route is logged in the contract's brace spelling, like the access log.
	for _, want := range []string{`"method":"GET"`, `"route":"/api/v1/orgs/{orgId}/apps/ragx/*path"`, `"jti":"jti-test"`, `"status":200`,
		`"duration_ms":`, `"request_id":"` + testRequestID + `"`, `"outcome":"success"`} {
		require.Contains(t, logs.String(), want)
	}
	token := strings.TrimPrefix((*requests)[0].header.Get("Authorization"), "Bearer ")
	require.NotContains(t, logs.String(), token)
	require.NotContains(t, logs.String(), "Bearer")
	require.NotContains(t, logs.String(), "secret-body-marker")
	require.Equal(t, []outboundCall{{"ragx", "success"}}, recorder.calls)
}
