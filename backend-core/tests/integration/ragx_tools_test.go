package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

func TestRagxTools_FakeRagxVerifiesTheToolToken(t *testing.T) {
	fake := newFakeRagx(t)
	logs := &bytes.Buffer{}
	caller := fake.caller(t, logs)
	kb := uuid.New()
	fake.setResponder(ragxJSON(http.StatusOK, `{"data":{"results":[],"insufficientEvidence":true},"meta":{"requestId":"x"}}`))
	_, err := caller.CallTool(orgContext("org-a"), kb, ragx.ToolKBSearch, json.RawMessage(`{"query":"x"}`))
	require.NoError(t, err)
	call := fake.calls()[0]
	require.Equal(t, "/v1/tools/kb_search", call.path)
	require.Equal(t, kb.String(), call.claims["kb"])
	require.Equal(t, "tool:kb_search", call.claims["scp"])
	require.Equal(t, "org-a", call.claims["oid"])
	require.Contains(t, logs.String(), `"msg":"tool.call"`)

	fake.setResponder(ragxError(http.StatusNotFound, "KB_NOT_FOUND"))
	_, err = caller.GetKnowledgeBase(orgContext("org-a"), kb)
	require.Equal(t, errors.CodeKbNotFound, errors.CodeOf(err))
}
