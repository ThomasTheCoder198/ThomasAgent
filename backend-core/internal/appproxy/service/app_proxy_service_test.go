package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

type fakeForwarder struct{ got []ragx.ForwardRequest }

func (f *fakeForwarder) Forward(_ context.Context, in ragx.ForwardRequest) (ragx.ForwardResponse, error) {
	f.got = append(f.got, in)
	return ragx.ForwardResponse{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{"data":{}}`)}, nil
}

func call(method, path string) model.AppCall {
	return model.AppCall{Method: method, Path: path, Route: "/api/v1/orgs/:orgId/apps/ragx/*path"}
}

func TestAppProxyService_ScopeFollowsTheMethod(t *testing.T) {
	for method, scope := range map[string]apptoken.Scope{
		http.MethodGet: apptoken.ScopeKBRead, http.MethodHead: apptoken.ScopeKBRead,
		http.MethodPost: apptoken.ScopeKBWrite, http.MethodPut: apptoken.ScopeKBWrite,
		http.MethodPatch: apptoken.ScopeKBWrite, http.MethodDelete: apptoken.ScopeKBWrite,
	} {
		forwarder := &fakeForwarder{}
		_, err := NewAppProxyService(forwarder).Forward(context.Background(), call(method, "/knowledge-bases"))
		require.NoError(t, err, method)
		require.Equal(t, scope, forwarder.got[0].Scope, method)
	}
	for _, method := range []string{http.MethodOptions, http.MethodTrace, http.MethodConnect, "PROPFIND"} {
		forwarder := &fakeForwarder{}
		_, err := NewAppProxyService(forwarder).Forward(context.Background(), call(method, "/knowledge-bases"))
		require.ErrorIs(t, err, errors.ErrMethodNotAllowed, method)
		require.Empty(t, forwarder.got)
	}
}

func TestAppProxyService_RejectsUnsafePathsWithoutForwarding(t *testing.T) {
	for _, raw := range []string{
		"", "/", "/knowledge-bases/", "/a//b", "//knowledge-bases", "/../orgs/x", "/knowledge-bases/..", "/kb/../x",
		"/.", "/knowledge-bases/.", "/./knowledge-bases", "/a/./b", "/a..b", "/kb x", "/kb%2Fx", "/kb;x", "/kb?x", "/kb#x", "/kb\\x", "/tài-liệu", "/kb\x00",
	} {
		forwarder := &fakeForwarder{}
		_, err := NewAppProxyService(forwarder).Forward(context.Background(), call(http.MethodGet, raw))
		require.ErrorIs(t, err, errors.ErrNotFound, "%q", raw)
		require.Empty(t, forwarder.got, "%q", raw)
	}
	for raw, want := range map[string]string{
		"/knowledge-bases": "knowledge-bases",
		"/knowledge-bases/8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f/files/abc-DEF_1.~x/download-url": "knowledge-bases/8f14e45f-ceea-467a-9af2-7a1b2c3d4e5f/files/abc-DEF_1.~x/download-url",
	} {
		forwarder := &fakeForwarder{}
		_, err := NewAppProxyService(forwarder).Forward(context.Background(), call(http.MethodGet, raw))
		require.NoError(t, err, raw)
		require.Equal(t, want, forwarder.got[0].Path, "the leading slash gin adds is removed")
	}
}

func TestAppProxyService_ValidatesTheIdempotencyKey(t *testing.T) {
	for _, key := range []string{"short", "has space 123", "ünïcode-key", strings.Repeat("a", 129)} {
		forwarder := &fakeForwarder{}
		in := call(http.MethodPost, "/knowledge-bases")
		in.IdempotencyKey = key
		_, err := NewAppProxyService(forwarder).Forward(context.Background(), in)
		require.ErrorIs(t, err, errors.ErrValidationFailed, key)
		require.Equal(t, []map[string]string{{"field": "Idempotency-Key", "code": errors.FieldCodeInvalid}}, errors.ToAppError(err).Details["fields"])
		require.Empty(t, forwarder.got)
	}
	forwarder := &fakeForwarder{}
	in := call(http.MethodPost, "/knowledge-bases")
	in.IdempotencyKey = "upl_3f2b8c1e-9d4a"
	_, err := NewAppProxyService(forwarder).Forward(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "upl_3f2b8c1e-9d4a", forwarder.got[0].IdempotencyKey)
}

func TestAppProxyService_PassesEverythingElseThrough(t *testing.T) {
	forwarder := &fakeForwarder{}
	in := model.AppCall{Method: http.MethodPatch, Path: "/knowledge-bases/k", RawQuery: "a=1", Route: "/r", ContentType: "application/json",
		AcceptLanguage: "en", Body: []byte(`{"name":"x"}`)}
	res, err := NewAppProxyService(forwarder).Forward(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, ragx.ForwardRequest{Method: http.MethodPatch, Path: "knowledge-bases/k", RawQuery: "a=1", Route: "/r",
		ContentType: "application/json", AcceptLanguage: "en", Body: []byte(`{"name":"x"}`), Scope: apptoken.ScopeKBWrite}, forwarder.got[0])
	require.Equal(t, model.AppResponse{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{"data":{}}`)}, res)
}
