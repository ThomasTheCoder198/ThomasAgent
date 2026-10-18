package integration

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/controller"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	agentservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/service"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const kbPathPrefix = "/v1/knowledge-bases/"

// kbResponder answers GET /v1/knowledge-bases/{id} like ragx: names for known ids, KB_NOT_FOUND otherwise.
func kbResponder(names map[uuid.UUID]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, kbPathPrefix))
		name, ok := names[id]
		if err != nil || !ok {
			ragxError(http.StatusNotFound, "KB_NOT_FOUND")(w, r)
			return
		}
		writeJSONBody(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id.String(), "name": name, "isDefault": false,
			"embeddingModel": "m", "embeddingDims": 1536, "fileCount": 1, "createdAt": "2026-10-10T00:00:00Z", "updatedAt": "2026-10-10T00:00:00Z"},
			"meta": map[string]any{"requestId": r.Header.Get("X-Request-Id")}})
	}
}

type agentFixture struct {
	router *gin.Engine
	llm    *llmFixture
	ragx   *fakeRagx
	logs   *bytes.Buffer
}

func newAgentFixture(t *testing.T) *agentFixture {
	t.Helper()
	llm := newLLMFixture(t)
	seedReadyOrg(t, llm.pool, "org-a")
	seedReadyOrg(t, llm.pool, "org-b")
	fake := newFakeRagx(t)
	logs := &bytes.Buffer{}
	log := logger(t, logs)
	service := agentservice.NewAgentService(llm.pool, agentrepository.NewAgentRepo(), fake.caller(t, logs), newResolveService(llm), log)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(log), httpx.LogAccess(log))
	router.Use(func(ctx *gin.Context) { // stands in for RequireOrg
		ctx.Request = ctx.Request.WithContext(tenant.WithID(ctx.Request.Context(), ctx.Param("orgId")))
		ctx.Next()
	})
	mountAgentsForTest(router, agentcontroller.NewAgentController(service, testMaxBodyBytes))
	return &agentFixture{router: router, llm: llm, ragx: fake, logs: logs}
}

// mountAgentsForTest registers the agent routes exactly as cmd/core/routes.go does (inside RequireOrg there).
func mountAgentsForTest(routes gin.IRoutes, c *agentcontroller.AgentController) {
	routes.GET("/api/v1/orgs/:orgId/agents", c.HandleListAgents)
	routes.POST("/api/v1/orgs/:orgId/agents", c.HandleCreateAgent)
	routes.GET("/api/v1/orgs/:orgId/agents/:agentId", c.HandleGetAgent)
	routes.PATCH("/api/v1/orgs/:orgId/agents/:agentId", c.HandleUpdateAgent)
	routes.DELETE("/api/v1/orgs/:orgId/agents/:agentId", c.HandleDeleteAgent)
}

func TestAgentHTTP_CreateChecksTheKBAndReadsItsName(t *testing.T) {
	f := newAgentFixture(t)
	kb, gone := uuid.New(), uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "Hợp đồng"}))

	res := send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"Hỗ trợ","kbId":"`+gone.String()+`"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.Code)
	require.Equal(t, "AGENT_KB_INVALID", errorOf(t, res)["code"])

	res = send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"Hỗ trợ","instructions":"Trả lời ngắn.","kbId":"`+kb.String()+`"}`, nil)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	agent := dataOf(t, res)
	require.Equal(t, "Hợp đồng", agent["kbName"])
	require.Equal(t, false, agent["kbMissing"])
	require.Equal(t, map[string]any{}, agent["modelOverrides"])
	calls := f.ragx.calls()
	require.Equal(t, "kb:read", calls[len(calls)-1].claims["scp"])
	require.Equal(t, "org-a", calls[len(calls)-1].claims["oid"])

	res = send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"Hỗ trợ","kbId":"`+kb.String()+`"}`, nil)
	require.Equal(t, http.StatusConflict, res.Code, "names are unique per org")
}

func TestAgentHTTP_CreateAnswersUnavailableWhenRagxIsDown(t *testing.T) {
	f := newAgentFixture(t)
	f.ragx.setResponder(ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"))

	res := send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+uuid.NewString()+`"}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, res.Code, "an outage is RAGX_UNAVAILABLE (a 5xx body hides the code), not an invalid KB: "+res.Body.String())
	var count int
	require.NoError(t, f.llm.pool.QueryRow(context.Background(), `SELECT count(*) FROM agents`).Scan(&count))
	require.Zero(t, count, "nothing is stored when the KB could not be checked")
}

func TestAgentHTTP_ReadReportsAMissingKBWithoutFailing(t *testing.T) {
	f := newAgentFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "Hợp đồng"}))
	created := dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))

	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{}))
	list := listOf(t, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/agents", "", nil))
	require.Len(t, list, 1)
	row := list[0].(map[string]any)
	require.Equal(t, "", row["kbName"])
	require.Equal(t, true, row["kbMissing"])

	f.ragx.setResponder(ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"))
	res := send(f.router, http.MethodGet, "/api/v1/orgs/org-a/agents/"+created["id"].(string), "", nil)
	require.Equal(t, http.StatusOK, res.Code, "a ragx outage never hides the agent")
	require.Equal(t, false, dataOf(t, res)["kbMissing"], "an outage is not a deleted KB")
	require.Equal(t, "", dataOf(t, res)["kbName"])
	require.Contains(t, f.logs.String(), "agent kb lookup failed")
}

func TestAgentHTTP_ValidatesModelOverridesAgainstRoles(t *testing.T) {
	f := newAgentFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "KB"}))
	provider := seedProvider(t, f.llm)
	chatOnly := seedChatModel(t, f.llm, provider, "x/chat-only", llmmodel.CapabilityChat)
	withTools := seedChatModel(t, f.llm, provider, "x/tools", llmmodel.CapabilityChat, llmmodel.CapabilityTools)

	res := send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents",
		`{"name":"A","kbId":"`+kb.String()+`","modelOverrides":{"chat.default":"`+chatOnly.String()+`"}}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "modelOverrides.chat.default")
	res = send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents",
		`{"name":"A","kbId":"`+kb.String()+`","modelOverrides":{"embedding":"`+withTools.String()+`"}}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code, "only chat.default, chat.fast and vision can be pinned")

	res = send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents",
		`{"name":"A","kbId":"`+kb.String()+`","modelOverrides":{"chat.default":"`+withTools.String()+`"}}`, nil)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	id := dataOf(t, res)["id"].(string)
	res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+id, `{"modelOverrides":{"chat.default":null}}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Equal(t, map[string]any{}, dataOf(t, res)["modelOverrides"])

	res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+id, `{"modelOverrides":{"chat.default":"`+chatOnly.String()+`"}}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code, "a patch checks the role fit like create")
	require.Contains(t, res.Body.String(), "modelOverrides.chat.default")
	res = send(f.router, http.MethodGet, "/api/v1/orgs/org-a/agents/"+id, "", nil)
	require.Equal(t, map[string]any{}, dataOf(t, res)["modelOverrides"], "a refused patch changes nothing")

	for range 20 { // map order is random, the reported field must not be
		res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+id, `{"modelOverrides":{"rerank":null,"embedding":null,"decision":null}}`, nil)
		require.Equal(t, http.StatusBadRequest, res.Code)
		require.Contains(t, res.Body.String(), "modelOverrides.decision", "the first invalid role in sorted order is reported")
	}
}

func TestAgentHTTP_PatchRechecksAChangedKBAndDeleteIsScoped(t *testing.T) {
	f := newAgentFixture(t)
	kb, other := uuid.New(), uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "KB"}))
	id := dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string)

	res := send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+id, `{"kbId":"`+other.String()+`"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.Code)
	res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+id, `{"name":"B"}`, nil)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "B", dataOf(t, res)["name"])

	require.Equal(t, http.StatusNotFound, send(f.router, http.MethodGet, "/api/v1/orgs/org-b/agents/"+id, "", nil).Code)
	require.Equal(t, http.StatusNotFound, send(f.router, http.MethodDelete, "/api/v1/orgs/org-b/agents/"+id, "", nil).Code)
	require.Equal(t, http.StatusNotFound, send(f.router, http.MethodGet, "/api/v1/orgs/org-a/agents/not-a-uuid", "", nil).Code)
	require.Equal(t, http.StatusNoContent, send(f.router, http.MethodDelete, "/api/v1/orgs/org-a/agents/"+id, "", nil).Code)
	rows, err := f.llm.pool.Query(context.Background(), `SELECT action, actor, tenant_id FROM audit_events WHERE target_id = $1 ORDER BY created_at, id`, id)
	require.NoError(t, err)
	defer rows.Close()
	type auditRow struct{ action, actor, tenantID string }
	var got []auditRow
	for rows.Next() {
		var r auditRow
		require.NoError(t, rows.Scan(&r.action, &r.actor, &r.tenantID))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []auditRow{{"agent.create", "owner", "org-a"}, {"agent.update", "owner", "org-a"}, {"agent.delete", "owner", "org-a"}}, got,
		"create, update and delete are audited under the org; a refused patch is not")
}

func TestAgentHTTP_PatchRefusesBadInputBeforeCallingRagx(t *testing.T) {
	f := newAgentFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "KB"}))
	first := dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string)
	second := dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"B","kbId":"`+kb.String()+`"}`, nil))["id"].(string)

	res := send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+second, `{"name":"A"}`, nil)
	require.Equal(t, http.StatusConflict, res.Code, "a rename onto an existing name conflicts like create")
	require.Equal(t, "CONFLICT", errorOf(t, res)["code"])

	before := len(f.ragx.calls())
	res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+first, `{"kbId":"`+uuid.Nil.String()+`"}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code, "the zero id is the same REQUIRED error create answers")
	require.Contains(t, res.Body.String(), "kbId")
	require.Len(t, f.ragx.calls(), before, "ragx is not asked about the zero id")

	f.ragx.setResponder(ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"))
	res = send(f.router, http.MethodPatch, "/api/v1/orgs/org-a/agents/"+first, `{"kbId":"`+uuid.NewString()+`"}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, res.Code, res.Body.String())
}
