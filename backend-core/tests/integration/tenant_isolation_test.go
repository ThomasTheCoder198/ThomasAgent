package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"

	agentcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/controller"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	agentservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/service"
	convcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/controller"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	convrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	llmservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	orgmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/middleware"
	orgrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	orgservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type chatAPIFixture struct {
	agentFixture
	store convservice.ChatStore
	user  uuid.UUID
}

func newChatAPIFixture(t *testing.T) *chatAPIFixture {
	t.Helper()
	base := newAgentFixture(t)
	pool := base.llm.pool
	userID := seedUser(t, pool, "owner@test.local")
	paging := config.ConversationConfig{PageDefaultLimit: 2, PageMaxLimit: 5}
	conversations := convservice.NewConversationService(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(),
		agentrepository.NewAgentRepo(), base.ragx.caller(t, base.logs), paging, logger(t, base.logs))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger(t, base.logs)))
	router.Use(func(ctx *gin.Context) {
		requestUser := userID
		if header := ctx.GetHeader("X-Test-User-Id"); header != "" {
			requestUser = uuid.MustParse(header)
		}
		ctx.Request = ctx.Request.WithContext(authmiddleware.WithUser(tenant.WithID(ctx.Request.Context(), ctx.Param("orgId")), authmodel.User{ID: requestUser}))
		ctx.Next()
	})
	service := agentservice.NewAgentService(pool, agentrepository.NewAgentRepo(), base.ragx.caller(t, base.logs), newResolveService(base.llm), logger(t, base.logs))
	mountAgentsForTest(router, agentcontroller.NewAgentController(service, testMaxBodyBytes))
	mountConversationsForTest(router, convcontroller.NewConversationController(conversations, paging, testMaxBodyBytes))
	base.router = router
	return &chatAPIFixture{agentFixture: *base, store: newChatStore(pool), user: userID}
}

// mountConversationsForTest registers the conversation routes exactly as cmd/core/routes.go does (inside RequireOrg there).
func mountConversationsForTest(routes gin.IRoutes, c *convcontroller.ConversationController) {
	routes.GET("/api/v1/orgs/:orgId/conversations", c.HandleListConversations)
	routes.GET("/api/v1/orgs/:orgId/conversations/:conversationId", c.HandleGetConversation)
	routes.PATCH("/api/v1/orgs/:orgId/conversations/:conversationId", c.HandleRenameConversation)
	routes.DELETE("/api/v1/orgs/:orgId/conversations/:conversationId", c.HandleDeleteConversation)
}

func TestTenantIsolation_AgentsConversationsRuns(t *testing.T) {
	f := newChatAPIFixture(t)
	kb := uuid.New()
	f.ragx.setResponder(kbResponder(map[uuid.UUID]string{kb: "KB"}))
	agentID := dataOf(t, send(f.router, http.MethodPost, "/api/v1/orgs/org-a/agents", `{"name":"A","kbId":"`+kb.String()+`"}`, nil))["id"].(string)
	ctxA, ctxB := conversationContext("org-a", f.user), conversationContext("org-b", f.user)
	conv := uuid.New()
	_, _, err := f.store.EnsureConversation(ctxA, conv, uuid.MustParse(agentID), f.user)
	require.NoError(t, err)
	require.NoError(t, f.store.AppendUserMessage(ctxA, conv, userText("u1", "bí mật của A")))
	run := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "a1", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.NoError(t, f.store.StartRun(ctxA, run))

	require.Empty(t, listOf(t, send(f.router, http.MethodGet, "/api/v1/orgs/org-b/agents", "", nil)))
	require.Empty(t, listOf(t, send(f.router, http.MethodGet, "/api/v1/orgs/org-b/conversations", "", nil)))
	for _, req := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/orgs/org-b/agents/" + agentID, ""},
		{http.MethodPatch, "/api/v1/orgs/org-b/agents/" + agentID, `{"name":"stolen"}`},
		{http.MethodDelete, "/api/v1/orgs/org-b/agents/" + agentID, ""},
		{http.MethodGet, "/api/v1/orgs/org-b/conversations/" + conv.String(), ""},
		{http.MethodPatch, "/api/v1/orgs/org-b/conversations/" + conv.String(), `{"title":"stolen"}`},
		{http.MethodDelete, "/api/v1/orgs/org-b/conversations/" + conv.String(), ""},
	} {
		res := send(f.router, req.method, req.path, req.body, nil)
		require.Equal(t, http.StatusNotFound, res.Code, req.method+" "+req.path)
		require.NotContains(t, res.Body.String(), "bí mật")
	}
	_, err = f.store.ActiveRun(ctxB, run.ID)
	require.Equal(t, errors.CodeRunNotActive, errors.CodeOf(err))
	_, _, err = f.store.EnsureConversation(ctxB, conv, seedAgent(t, f.llm.pool, "org-b", "B"), f.user)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err), "B cannot claim A's conversation id")
	history, err := f.store.LoadHistory(ctxB, conv, 10)
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(err))
	require.Empty(t, history.Messages)

	res := send(f.router, http.MethodGet, "/api/v1/orgs/org-a/conversations/"+conv.String(), "", nil)
	require.Equal(t, http.StatusOK, res.Code, "A still owns everything")
}

// TestTenantIsolation_ChatStoreWritesNeverCrossOrgs: every ChatStore writer under org B's context is refused for org
// A's conversation and run, and leaves A's rows exactly as they were.
func TestTenantIsolation_ChatStoreWritesNeverCrossOrgs(t *testing.T) {
	f := newChatAPIFixture(t)
	ctxA, ctxB := conversationContext("org-a", f.user), conversationContext("org-b", f.user)
	conv := uuid.New()
	_, _, err := f.store.EnsureConversation(ctxA, conv, seedAgent(t, f.llm.pool, "org-a", "A"), f.user)
	require.NoError(t, err)
	require.NoError(t, f.store.AppendUserMessage(ctxA, conv, userText("u1", "bí mật của A")))
	run := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "a1", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.NoError(t, f.store.StartRun(ctxA, run))

	foreignRun := convmodel.Run{ID: uuid.New(), ConversationID: conv, AssistantMessageID: "b1", Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(time.Minute)}
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.AppendUserMessage(ctxB, conv, userText("u2", "chèn vào"))))
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.StartRun(ctxB, foreignRun)))
	require.Equal(t, errors.CodeRunNotActive, errors.CodeOf(f.store.RecordStep(ctxB, convmodel.RunStep{RunID: run.ID, Step: 1,
		Kind: convmodel.StepKindModel, Outcome: convmodel.StepOutcomeSuccess, DurationMS: 1, StartedAt: time.Now()})))
	run.Status = convmodel.RunStatusCancelled
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.FinishRun(ctxB, run,
		convmodel.Message{ID: "a1", Role: convmodel.RoleAssistant, Parts: []byte(`[{"type":"text","text":"x"}]`)})))
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.SaveSummary(ctxB, conv, "tóm tắt giả", 1)))
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.SetTitle(ctxB, conv, "stolen")))
	require.Equal(t, errors.CodeConversationNotFound, errors.CodeOf(f.store.DropLastAssistant(ctxB, conv, "a1")))

	history, err := f.store.LoadHistory(ctxA, conv, 10)
	require.NoError(t, err)
	require.Len(t, history.Messages, 1, "B added no message")
	require.Equal(t, "u1", history.Messages[0].ID)
	require.Empty(t, history.Conversation.Title)
	require.Empty(t, history.Conversation.Summary)
	require.Zero(t, history.Conversation.SummaryThroughSeq)
	active, err := f.store.ActiveRun(ctxA, run.ID)
	require.NoError(t, err)
	require.Equal(t, convmodel.RunStatusRunning, active.Status, "B did not finish A's run")
	var runs, steps int
	require.NoError(t, f.llm.pool.QueryRow(context.Background(), `SELECT count(*) FROM runs`).Scan(&runs))
	require.NoError(t, f.llm.pool.QueryRow(context.Background(), `SELECT count(*) FROM run_steps`).Scan(&steps))
	require.Equal(t, 1, runs)
	require.Zero(t, steps)
}

// requireOrgRouter mounts the org-role, agent and conversation routes the way cmd/core/routes.go does: the session
// stand-in sets the platform tenant, then the real RequireOrg turns the path org into the tenant. Org ids must be
// UUIDs, as in core.
func requireOrgRouter(t *testing.T, f *chatAPIFixture) *gin.Engine {
	t.Helper()
	pool := f.llm.pool
	log := logger(t, f.logs)
	paging := config.ConversationConfig{PageDefaultLimit: 2, PageMaxLimit: 5}
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	organizations := orgservice.NewOrganizationService(pool, orgrepository.NewOrganizationRepo(), f.ragx.caller(t, f.logs), policy, uuid.NewString)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(log))
	session := router.Group("", func(ctx *gin.Context) { // stands in for RequireSession
		ctx.Request = ctx.Request.WithContext(authmiddleware.WithUser(platformContext(ctx.Request.Context()), authmodel.User{ID: f.user}))
		ctx.Next()
	})
	orgScoped := session.Group("", orgmiddleware.RequireOrg(organizations))
	mountOrgRolesForTest(orgScoped, llmcontroller.NewOrgRoleController(
		llmservice.NewOrgRoleService(pool, llmrepository.NewLLMModelRepo(llmrepository.NewProviderRepo()), llmrepository.NewRoleRepo()), testMaxBodyBytes))
	mountAgentsForTest(orgScoped, agentcontroller.NewAgentController(
		agentservice.NewAgentService(pool, agentrepository.NewAgentRepo(), f.ragx.caller(t, f.logs), newResolveService(f.llm), log), testMaxBodyBytes))
	conversations := convservice.NewConversationService(pool, convrepository.NewConversationRepo(), convrepository.NewMessageRepo(),
		agentrepository.NewAgentRepo(), f.ragx.caller(t, f.logs), paging, log)
	mountConversationsForTest(orgScoped, convcontroller.NewConversationController(conversations, paging, testMaxBodyBytes))
	return router
}

func seedOrgWithStatus(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `INSERT INTO organizations (id, tenant_id, name, external_parsing, ragx_status, settings_synced)
		VALUES ($1, $2, $1, false, $3, false)`, id, tenant.PlatformID, status)
	require.NoError(t, err)
	return id
}

// TestTenantIsolation_OrgRoutesGoThroughRequireOrg (ruling F2a2-3): the org-role, agent and conversation routes run
// only for a known, provisioned org; an unknown, platform or unprovisioned org never reaches a handler.
func TestTenantIsolation_OrgRoutesGoThroughRequireOrg(t *testing.T) {
	f := newChatAPIFixture(t)
	router := requireOrgRouter(t, f)
	ready := seedOrgWithStatus(t, f.llm.pool, "ready")
	pending := seedOrgWithStatus(t, f.llm.pool, "pending")
	routes := []struct{ method, suffix, body string }{
		{http.MethodGet, "/model-roles", ""},
		{http.MethodPut, "/model-roles/chat.default", `{"modelId":"` + uuid.NewString() + `"}`},
		{http.MethodDelete, "/model-roles/chat.default", ""},
		{http.MethodGet, "/agents", ""},
		{http.MethodPost, "/agents", `{"name":"X","kbId":"` + uuid.NewString() + `"}`},
		{http.MethodGet, "/agents/" + uuid.NewString(), ""},
		{http.MethodPatch, "/agents/" + uuid.NewString(), `{"name":"X"}`},
		{http.MethodDelete, "/agents/" + uuid.NewString(), ""},
		{http.MethodGet, "/conversations", ""},
		{http.MethodGet, "/conversations/" + uuid.NewString(), ""},
		{http.MethodPatch, "/conversations/" + uuid.NewString(), `{"title":"X"}`},
		{http.MethodDelete, "/conversations/" + uuid.NewString(), ""},
	}
	refused := map[string]struct {
		status int
		code   errors.Code
	}{
		uuid.NewString():  {http.StatusNotFound, errors.CodeNotFound},
		"org-a":           {http.StatusNotFound, errors.CodeNotFound},
		tenant.PlatformID: {http.StatusNotFound, errors.CodeNotFound},
		pending:           {http.StatusConflict, errors.CodeOrgNotProvisioned},
	}
	for _, route := range routes {
		for org, want := range refused {
			label := route.method + " " + route.suffix + " org=" + org
			res := send(router, route.method, "/api/v1/orgs/"+org+route.suffix, route.body, nil)
			require.Equal(t, want.status, res.Code, label)
			require.Equal(t, string(want.code), errorOf(t, res)["code"], label)
		}
	}
	for _, table := range []string{"model_roles", "agents", "conversations"} {
		var count int
		require.NoError(t, f.llm.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, pending).Scan(&count))
		require.Zero(t, count, "nothing is written for an unprovisioned org: "+table)
	}

	for _, suffix := range []string{"/model-roles", "/agents", "/conversations"} {
		res := send(router, http.MethodGet, "/api/v1/orgs/"+ready+suffix, "", nil)
		require.Equal(t, http.StatusOK, res.Code, suffix+": "+res.Body.String())
	}
}
