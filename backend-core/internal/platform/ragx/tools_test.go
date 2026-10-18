package ragx

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
)

type fakeToolLog struct{ calls []calllog.ToolCall }

func (f *fakeToolLog) Tool(_ context.Context, call calllog.ToolCall) { f.calls = append(f.calls, call) }

var testKB = uuid.MustParse("1a2c82b9-facd-4f76-894b-1b7151eaf0b5")

func TestCallTool_StripsScopeKeysAndSignsKB(t *testing.T) {
	caller, requests, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":{"results":[],"insufficientEvidence":true},"meta":{"requestId":"x"}}`)
	})
	data, err := caller.CallTool(orgContext(), testKB, ToolKBSearch,
		json.RawMessage(`{"query":"giá gói Pro","kbId":"other","KB_ID":"x","KbId":"y","orgId":"b","ORG_ID":"z","tenant_id":"c","TenantId":"d"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"results":[],"insufficientEvidence":true}`, string(data))
	got := (*requests)[0]
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/v1/tools/kb_search", got.path)
	require.JSONEq(t, `{"query":"giá gói Pro"}`, string(got.body), "scope keys never reach ragx (A2-7)")
	require.Equal(t, testOrg, got.claims["oid"])
	require.Equal(t, testKB.String(), got.claims["kb"])
	require.Equal(t, "tool:kb_search", got.claims["scp"])
	require.Equal(t, "application/json", got.header.Get("Content-Type"))
	require.Empty(t, got.header.Values("Idempotency-Key"), "the repeatable marker is never written")
}

func TestIsScopeKey_IgnoresCase(t *testing.T) {
	for _, key := range []string{"kbId", "KBID", "kb_id", "KB_ID", "orgId", "Org_Id", "tenantId", "TENANT_ID"} {
		require.True(t, IsScopeKey(key), key)
	}
	for _, key := range []string{"query", "fileId", "kb", "org"} {
		require.False(t, IsScopeKey(key), key)
	}
}

func TestCallTool_LogsOneToolCallWithTraceID(t *testing.T) {
	toolLog := &fakeToolLog{}
	caller, _, _, logs := newCallerWithToolLog(t, toolLog, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":{"nodes":[]},"meta":{"requestId":"x"}}`)
	})
	_, err := caller.CallTool(orgContext(), testKB, ToolKBOutline, json.RawMessage(`{"fileId":"`+uuid.NewString()+`"}`))
	require.NoError(t, err)
	require.Len(t, toolLog.calls, 1)
	require.Equal(t, calllog.ToolCall{App: appRagx, Tool: "kb_outline", Duration: toolLog.calls[0].Duration,
		RagxTraceID: "0af7651916cd43dd8448eb211c80319c"}, toolLog.calls[0])
	require.Contains(t, logs.String(), `"msg":"ragx call"`)
}

func TestCallTool_PassesKBNotFoundAndMapsFailures(t *testing.T) {
	toolLog := &fakeToolLog{}
	caller, _, _, _ := newCallerWithToolLog(t, toolLog, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"error":{"code":"KB_NOT_FOUND","message":"","requestId":"x"}}`)
	})
	_, err := caller.CallTool(orgContext(), testKB, ToolKBSearch, json.RawMessage(`{"query":"x"}`))
	require.Equal(t, errors.CodeKbNotFound, errors.CodeOf(err))
	require.Equal(t, string(errors.CodeKbNotFound), toolLog.calls[0].ErrorCode)

	caller, _, _, _ = newCallerWithToolLog(t, toolLog, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"","requestId":"x"}}`)
	})
	_, err = caller.CallTool(orgContext(), testKB, ToolKBSearch, json.RawMessage(`{"query":"x"}`))
	require.Equal(t, errors.CodeRagxUnavailable, errors.CodeOf(err), "a refused token is core's fault, not the operator's")
}

func TestCallTool_RefusesBadInputBeforeAnyRequest(t *testing.T) {
	caller, requests, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_, err := caller.CallTool(orgContext(), uuid.Nil, ToolKBSearch, json.RawMessage(`{"query":"x"}`))
	require.Equal(t, errors.CodeInternalError, errors.CodeOf(err))
	_, err = caller.CallTool(orgContext(), testKB, ToolKBSearch, json.RawMessage(`["not","an","object"]`))
	require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(err))
	require.Empty(t, *requests)
}

func TestCallTool_UsesTheToolTimeout(t *testing.T) {
	cfg := testConfig("")
	cfg.ToolTimeout = 50 * time.Millisecond
	caller, _ := slowCaller(t, cfg, 2*time.Second)
	start := time.Now()
	_, err := caller.CallTool(orgContext(), testKB, ToolKBSearch, json.RawMessage(`{"query":"x"}`))
	require.Equal(t, errors.CodeRagxUnavailable, errors.CodeOf(err))
	require.Less(t, time.Since(start), time.Second)
}

func TestGetKnowledgeBase_ReadsWithKBReadScope(t *testing.T) {
	caller, requests, _, _ := newCaller(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":{"id":"`+testKB.String()+`","name":"Hợp đồng","isDefault":false,"embeddingModel":"m","embeddingDims":1536,"fileCount":3,"createdAt":"2026-10-10T00:00:00Z","updatedAt":"2026-10-10T00:00:00Z"},"meta":{"requestId":"x"}}`)
	})
	kb, err := caller.GetKnowledgeBase(orgContext(), testKB)
	require.NoError(t, err)
	require.Equal(t, "Hợp đồng", kb.Name)
	require.Equal(t, "/v1/knowledge-bases/"+testKB.String(), (*requests)[0].path)
	require.Equal(t, "kb:read", (*requests)[0].claims["scp"])
	require.Nil(t, (*requests)[0].claims["kb"], "a kb:read token carries no kb claim")
}
