package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestConversationHTTP_ListPagesGetRenameDelete(t *testing.T) {
	f := newChatAPIFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "Hợp đồng"}))
	agentID := uuid.MustParse(dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string))
	ctx := conversationContext("org-a", f.user)
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range ids {
		_, _, err := f.store.EnsureConversation(ctx, id, agentID, f.user)
		require.NoError(t, err)
		appendUserMessage(t, f.llm.pool, ctx, id, userText("u-"+id.String(), "hi"))
	}

	first := send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations", "", nil)
	require.Equal(t, http.StatusOK, first.Code)
	var page struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Page struct {
				Cursor *string `json:"cursor"`
				Limit  int     `json:"limit"`
			} `json:"page"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Data, 2, "default limit from config")
	require.Equal(t, ids[2].String(), page.Data[0]["id"], "newest first")
	require.Equal(t, []any{"kb"}, page.Data[0]["lines"])
	require.NotNil(t, page.Meta.Page.Cursor)
	second := send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations?cursor="+url.QueryEscape(*page.Meta.Page.Cursor), "", nil)
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	require.Nil(t, page.Meta.Page.Cursor, "last page")
	require.Equal(t, http.StatusBadRequest, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations?limit=500", "", nil).Code)
	require.Equal(t, http.StatusBadRequest, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations?cursor=%21%21", "", nil).Code)

	detail := dataOf(t, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations/"+ids[0].String(), "", nil))
	scope := detail["scope"].(map[string]any)
	require.Equal(t, "Hợp đồng", scope["kb"].(map[string]any)["name"])
	require.Equal(t, false, scope["kb"].(map[string]any)["missing"])
	require.Equal(t, []any{map[string]any{"line": "kb", "name": "Hợp đồng"}}, scope["toolSources"])
	messages := detail["messages"].([]any)
	require.Equal(t, []any{map[string]any{"type": "text", "text": "hi"}}, messages[0].(map[string]any)["parts"])

	require.Equal(t, http.StatusBadRequest, send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/conversations/"+ids[0].String(), `{"title":"  "}`, nil).Code)
	res := send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/conversations/"+ids[0].String(), `{"title":"Giá gói Pro"}`, nil)
	require.Equal(t, http.StatusNoContent, res.Code)
	require.Equal(t, http.StatusNoContent, send(f.router, http.MethodDelete, "/api/v1/orgs/org-a/conversations/"+ids[0].String(), "", nil).Code)
	require.Equal(t, http.StatusNotFound, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations/"+ids[0].String(), "", nil).Code)
	require.Equal(t, http.StatusNotFound, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations/not-a-uuid", "", nil).Code)
}

func TestConversationHTTP_ScopeSeparatesADeletedKBFromAnOutage(t *testing.T) {
	f := newChatAPIFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "Hợp đồng"}))
	agentID := uuid.MustParse(dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string))
	conv := uuid.New()
	_, _, err := f.store.EnsureConversation(conversationContext("org-a", f.user), conv, agentID, f.user)
	require.NoError(t, err)
	scopeOf := func() map[string]any {
		res := send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations/"+conv.String(), "", nil)
		require.Equal(t, http.StatusOK, res.Code, "a KB lookup never fails the read: "+res.Body.String())
		return dataOf(t, res)["scope"].(map[string]any)
	}

	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{}))
	scope := scopeOf()
	require.Equal(t, true, scope["kb"].(map[string]any)["missing"], "KB_NOT_FOUND is a deleted KB")
	require.Equal(t, "", scope["kb"].(map[string]any)["name"])
	require.Equal(t, []any{}, scope["toolSources"], "a deleted KB leaves no KB tools")

	f.ragx.setResponder(ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"))
	scope = scopeOf()
	require.Equal(t, false, scope["kb"].(map[string]any)["missing"], "an outage is not a deleted KB (index amendment 18)")
	require.Equal(t, "", scope["kb"].(map[string]any)["name"])
	require.Equal(t, []any{map[string]any{"line": "kb", "name": ""}}, scope["toolSources"], "the run keeps its KB tools")
	require.Equal(t, agentID.String(), scope["agent"].(map[string]any)["id"])
}

func TestConversationHTTP_SameOrgDifferentUserCannotReadOrChange(t *testing.T) {
	f := newChatAPIFixture(t)
	other := seedUser(t, f.llm.pool, "other@test.local")
	agent := seedAgent(t, f.llm.pool, "org-a", "A")
	id := uuid.New()
	_, _, err := f.store.EnsureConversation(conversationContext("org-a", f.user), id, agent, f.user)
	require.NoError(t, err)
	appendUserMessage(t, f.llm.pool, conversationContext("org-a", f.user), id, userText("u1", "private"))
	headers := map[string]string{"X-Test-User-Id": other.String()}
	require.Empty(t, listOf(t, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations", "", headers)))
	path := "/api/v1/orgs/org-a/conversations/" + id.String()
	for _, request := range []struct{ method, body string }{
		{http.MethodGet, ""}, {http.MethodPatch, `{"title":"stolen"}`}, {http.MethodDelete, ""},
	} {
		res := send(f.router, request.method, path, request.body, headers)
		require.Equal(t, http.StatusNotFound, res.Code)
		require.NotContains(t, res.Body.String(), "private")
	}
	require.Equal(t, http.StatusOK, send(f.router, http.MethodGet, path, "", nil).Code)
}

// TestConversationHTTP_LastRunIsNullThenFollowsTheLatestRun: the detail's lastRun is how a reopened chat learns that
// its last answer stopped early (amendment W7), and the request id lets support find the run's logs.
func TestConversationHTTP_LastRunIsNullThenFollowsTheLatestRun(t *testing.T) {
	f := newChatAPIFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "Hợp đồng"}))
	agentID := uuid.MustParse(dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string))
	ctx, firstRequestID := withRequestID(conversationContext("org-a", f.user))
	conv := uuid.New()
	_, _, err := f.store.EnsureConversation(ctx, conv, agentID, f.user)
	require.NoError(t, err)
	path := "/api/v1/orgs/org-a/conversations/" + conv.String()
	lastRun := func() any {
		res := send(f.router, http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusOK, res.Code)
		detail := dataOf(t, res)
		require.Contains(t, detail, "lastRun", "the key is always present: null means no run yet")
		return detail["lastRun"]
	}

	require.Nil(t, lastRun(), "a conversation without runs")

	first, _, err := f.store.BeginRun(ctx, submitInput(conv, agentID, "u1", "hỏi", "a1"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"id": first.ID.String(), "status": "running", "errorCode": "", "requestId": firstRequestID}, lastRun())

	flipped, err := f.store.FailRun(ctx, first.ID, "RUN_INTERRUPTED", false)
	require.NoError(t, err)
	require.True(t, flipped)
	require.Equal(t, map[string]any{"id": first.ID.String(), "status": "failed", "errorCode": "RUN_INTERRUPTED", "requestId": firstRequestID}, lastRun())

	secondCtx, secondRequestID := withRequestID(conversationContext("org-a", f.user))
	second, _, err := f.store.BeginRun(secondCtx, regenerateInput(conv, agentID, "u1", "a1"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"id": second.ID.String(), "status": "running", "errorCode": "", "requestId": secondRequestID}, lastRun(),
		"the latest run wins")

	other := seedUser(t, f.llm.pool, "other@test.local")
	res := send(f.router, http.MethodGet, path, "", map[string]string{"X-Test-User-Id": other.String()})
	require.Equal(t, http.StatusNotFound, res.Code, "another user's conversation is not found")
	require.NotContains(t, res.Body.String(), second.ID.String())
}
