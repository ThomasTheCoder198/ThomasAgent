package integration

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/controller"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	llmservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// seedChatModel adds a platform model with the given capabilities to provider and returns its id.
func seedChatModel(t *testing.T, fixture *llmFixture, provider llmmodel.Provider, ref string, caps ...llmmodel.Capability) uuid.UUID {
	t.Helper()
	m, err := fixture.models.CreateModel(platformContext(context.Background()), llmmodel.LLMModelInput{
		ProviderID: provider.ID, ModelRef: ref, DisplayName: ref, Capabilities: caps})
	require.NoError(t, err)
	return m.ID
}

func newResolveService(fixture *llmFixture) llmservice.ResolveService {
	providerRepo := llmrepository.NewProviderRepo()
	return llmservice.NewResolveService(fixture.pool, fixture.providers, llmrepository.NewLLMModelRepo(providerRepo), llmrepository.NewRoleRepo())
}

func TestResolve_OrgContextFindsPlatformModel(t *testing.T) {
	fixture := newLLMFixture(t)
	seedReadyOrg(t, fixture.pool, "org-a")
	provider := seedProvider(t, fixture)
	fast := seedChatModel(t, fixture, provider, "openai/gpt-4.1-nano", llmmodel.CapabilityChat)
	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatFast, fast))
	resolver := newResolveService(fixture)

	resolved, err := resolver.Resolve(orgContext("org-a"), llmmodel.RoleChatFast)
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-4.1-nano", resolved.ModelRef)
	require.Equal(t, llmmodel.ResolvedSourcePlatform, resolved.Source)
	require.Equal(t, provider.ID, resolved.ProviderID)
	require.Equal(t, providerSecretKey, resolved.APIKey)
	require.NotEmpty(t, resolved.KeyVersion)

	decision, err := resolver.Resolve(orgContext("org-a"), llmmodel.RoleDecision)
	require.NoError(t, err)
	require.Equal(t, llmmodel.ResolvedSourceFallback, decision.Source)
	require.Equal(t, llmmodel.RoleDecision, decision.FallbackFrom)
	require.Equal(t, "openai/gpt-4.1-nano", decision.ModelRef)

	byID, err := resolver.ResolveModel(orgContext("org-a"), fast)
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-4.1-nano", byID.ModelRef)
}

func newOrgRoleRouter(t *testing.T, fixture *llmFixture) *gin.Engine {
	t.Helper()
	logs := &bytes.Buffer{}
	logger, err := logging.NewLogger(logs, "debug", "test", "test", nil)
	require.NoError(t, err)
	providerRepo := llmrepository.NewProviderRepo()
	service := llmservice.NewOrgRoleService(fixture.pool, llmrepository.NewLLMModelRepo(providerRepo), llmrepository.NewRoleRepo())
	controller := llmcontroller.NewOrgRoleController(service, testMaxBodyBytes)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	// Stands in for RequireOrg: the path org becomes the tenant (org rows are seeded ready).
	router.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(tenant.WithID(ctx.Request.Context(), ctx.Param("orgId")))
		ctx.Next()
	})
	mountOrgRolesForTest(router, controller)
	return router
}

// mountOrgRolesForTest registers the org role routes exactly as cmd/core/routes.go does (inside RequireOrg there).
func mountOrgRolesForTest(routes gin.IRoutes, controller *llmcontroller.OrgRoleController) {
	routes.GET("/api/v1/orgs/:orgId/model-roles", controller.HandleListOrgRoles)
	routes.PUT("/api/v1/orgs/:orgId/model-roles/:role", controller.HandleAssignOrgRole)
	routes.DELETE("/api/v1/orgs/:orgId/model-roles/:role", controller.HandleClearOrgRole)
}

func roleRow(t *testing.T, res []any, role string) map[string]any {
	t.Helper()
	for _, item := range res {
		row := item.(map[string]any)
		if row["role"] == role {
			return row
		}
	}
	t.Fatalf("role %s missing", role)
	return nil
}

func TestOrgRoles_ClearFallsBackToPlatform(t *testing.T) {
	fixture := newLLMFixture(t)
	seedReadyOrg(t, fixture.pool, "org-a")
	provider := seedProvider(t, fixture)
	platformModel := seedChatModel(t, fixture, provider, "anthropic/claude-sonnet-4.5", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	orgModel := seedChatModel(t, fixture, provider, "openai/gpt-5-mini", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatDefault, platformModel))
	router := newOrgRoleRouter(t, fixture)
	resolver := newResolveService(fixture)

	res := send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/chat.default", `{"modelId":"`+orgModel.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	resolved, err := resolver.Resolve(orgContext("org-a"), llmmodel.RoleChatDefault)
	require.NoError(t, err)
	require.Equal(t, llmmodel.ResolvedSourceOrg, resolved.Source)
	require.Equal(t, "openai/gpt-5-mini", resolved.ModelRef)

	list := listOf(t, send(router, http.MethodGet, "/api/v1/orgs/org-a/model-roles", "", nil))
	require.Len(t, list, 4)
	row := roleRow(t, list, "chat.default")
	require.Equal(t, orgModel.String(), row["modelId"])
	require.Equal(t, "org", row["source"])
	vision := roleRow(t, list, "vision")
	require.Nil(t, vision["modelId"])
	require.Nil(t, vision["effectiveModelId"])
	require.Equal(t, "none", vision["source"])

	res = send(router, http.MethodDelete, "/api/v1/orgs/org-a/model-roles/chat.default", "", nil)
	require.Equal(t, http.StatusNoContent, res.Code)
	resolved, err = resolver.Resolve(orgContext("org-a"), llmmodel.RoleChatDefault)
	require.NoError(t, err)
	require.Equal(t, llmmodel.ResolvedSourcePlatform, resolved.Source)
	row = roleRow(t, listOf(t, send(router, http.MethodGet, "/api/v1/orgs/org-a/model-roles", "", nil)), "chat.default")
	require.Nil(t, row["modelId"])
	require.Equal(t, platformModel.String(), row["effectiveModelId"])
	require.Equal(t, "platform", row["source"])
}

// auditTargets returns the target ids of every audit row with action, oldest first.
func auditTargets(t *testing.T, pool *pgxpool.Pool, action string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT target_id FROM audit_events WHERE action = $1 ORDER BY created_at, id`, action)
	require.NoError(t, err)
	defer rows.Close()
	targets := []string{}
	for rows.Next() {
		var target string
		require.NoError(t, rows.Scan(&target))
		targets = append(targets, target)
	}
	require.NoError(t, rows.Err())
	return targets
}

func TestOrgRoles_OverridesAreIsolatedPerOrgAndAudited(t *testing.T) {
	fixture := newLLMFixture(t)
	seedReadyOrg(t, fixture.pool, "org-a")
	seedReadyOrg(t, fixture.pool, "org-b")
	provider := seedProvider(t, fixture)
	platformModel := seedChatModel(t, fixture, provider, "anthropic/claude-sonnet-4.5", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	orgAModel := seedChatModel(t, fixture, provider, "openai/gpt-5-mini", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	orgBModel := seedChatModel(t, fixture, provider, "google/gemini-2.5-flash", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatDefault, platformModel))
	router := newOrgRoleRouter(t, fixture)
	resolver := newResolveService(fixture)
	resolveRef := func(org string) (string, string) {
		t.Helper()
		resolved, err := resolver.Resolve(orgContext(org), llmmodel.RoleChatDefault)
		require.NoError(t, err)
		return resolved.ModelRef, resolved.Source
	}

	res := send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/chat.default", `{"modelId":"`+orgAModel.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	ref, source := resolveRef("org-a")
	require.Equal(t, "openai/gpt-5-mini", ref)
	require.Equal(t, llmmodel.ResolvedSourceOrg, source)
	ref, source = resolveRef("org-b")
	require.Equal(t, "anthropic/claude-sonnet-4.5", ref, "org A's override must not leak to org B")
	require.Equal(t, llmmodel.ResolvedSourcePlatform, source)
	rowB := roleRow(t, listOf(t, send(router, http.MethodGet, "/api/v1/orgs/org-b/model-roles", "", nil)), "chat.default")
	require.Nil(t, rowB["modelId"])
	require.Equal(t, platformModel.String(), rowB["effectiveModelId"])
	require.Equal(t, "platform", rowB["source"])
	require.Equal(t, []string{"org-a:chat.default"}, auditTargets(t, fixture.pool, "model_role.assign_org"))

	res = send(router, http.MethodPut, "/api/v1/orgs/org-b/model-roles/chat.default", `{"modelId":"`+orgBModel.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	res = send(router, http.MethodDelete, "/api/v1/orgs/org-a/model-roles/chat.default", "", nil)
	require.Equal(t, http.StatusNoContent, res.Code)
	ref, source = resolveRef("org-a")
	require.Equal(t, "anthropic/claude-sonnet-4.5", ref)
	require.Equal(t, llmmodel.ResolvedSourcePlatform, source)
	ref, source = resolveRef("org-b")
	require.Equal(t, "google/gemini-2.5-flash", ref, "clearing org A's override must not touch org B's")
	require.Equal(t, llmmodel.ResolvedSourceOrg, source)
	var roleRows int
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM model_roles WHERE role = 'chat.default'`).Scan(&roleRows))
	require.Equal(t, 2, roleRows, "the platform row and org B's row remain")

	require.Equal(t, []string{"org-a:chat.default", "org-b:chat.default"}, auditTargets(t, fixture.pool, "model_role.assign_org"))
	require.Equal(t, []string{"org-a:chat.default"}, auditTargets(t, fixture.pool, "model_role.clear_org"))
	res = send(router, http.MethodDelete, "/api/v1/orgs/org-a/model-roles/chat.default", "", nil)
	require.Equal(t, http.StatusNoContent, res.Code, "clearing an absent override is idempotent")
	require.Equal(t, []string{"org-a:chat.default", "org-a:chat.default"}, auditTargets(t, fixture.pool, "model_role.clear_org"),
		"each clear is audited, like each assign")
	require.Equal(t, []string{"chat.default"}, auditTargets(t, fixture.pool, "model_role.assign"),
		"only the platform setup is a platform assignment; org assignments are not recorded as one")
}

func TestOrgRoles_OrgChatFastWinsDecisionFallback(t *testing.T) {
	fixture := newLLMFixture(t)
	seedReadyOrg(t, fixture.pool, "org-a")
	seedReadyOrg(t, fixture.pool, "org-b")
	provider := seedProvider(t, fixture)
	platformFast := seedChatModel(t, fixture, provider, "openai/gpt-4.1-nano", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	orgFast := seedChatModel(t, fixture, provider, "openai/gpt-5-nano", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatFast, platformFast))
	router := newOrgRoleRouter(t, fixture)
	resolver := newResolveService(fixture)

	res := send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/chat.fast", `{"modelId":"`+orgFast.String()+`"}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	decision, err := resolver.Resolve(orgContext("org-a"), llmmodel.RoleDecision)
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-5-nano", decision.ModelRef, "the org's chat.fast beats the platform's for the decision fallback")
	require.Equal(t, llmmodel.ResolvedSourceFallback, decision.Source)
	require.Equal(t, llmmodel.RoleDecision, decision.FallbackFrom)
	row := roleRow(t, listOf(t, send(router, http.MethodGet, "/api/v1/orgs/org-a/model-roles", "", nil)), "decision")
	require.Nil(t, row["modelId"], "the org has no decision row of its own")
	require.Equal(t, orgFast.String(), row["effectiveModelId"], "the Settings list agrees with the resolver")
	require.Equal(t, "fallback", row["source"])

	other, err := resolver.Resolve(orgContext("org-b"), llmmodel.RoleDecision)
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-4.1-nano", other.ModelRef, "org B still falls back to the platform's chat.fast")
	require.Equal(t, llmmodel.ResolvedSourceFallback, other.Source)

	res = send(router, http.MethodDelete, "/api/v1/orgs/org-a/model-roles/chat.fast", "", nil)
	require.Equal(t, http.StatusNoContent, res.Code)
	decision, err = resolver.Resolve(orgContext("org-a"), llmmodel.RoleDecision)
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-4.1-nano", decision.ModelRef, "clearing the override returns the decision fallback to the platform's chat.fast")
}

func TestOrgRoles_RejectsRagxRolesAndUnfitModels(t *testing.T) {
	fixture := newLLMFixture(t)
	seedReadyOrg(t, fixture.pool, "org-a")
	provider := seedProvider(t, fixture)
	chatOnly := seedChatModel(t, fixture, provider, "x/chat-only", llmmodel.CapabilityChat)
	router := newOrgRoleRouter(t, fixture)

	res := send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/embedding", `{"modelId":"`+chatOnly.String()+`"}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code)
	res = send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/chat.default", `{"modelId":"`+chatOnly.String()+`"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.Code)
	require.Equal(t, "LLM_MODEL_CAPABILITY_MISMATCH", errorOf(t, res)["code"])
	res = send(router, http.MethodPut, "/api/v1/orgs/org-a/model-roles/chat.fast", `{"modelId":"`+uuid.NewString()+`"}`, nil)
	require.Equal(t, http.StatusNotFound, res.Code)
	res = send(router, http.MethodDelete, "/api/v1/orgs/org-a/model-roles/chat.fast", "", nil)
	require.Equal(t, http.StatusNoContent, res.Code, "clearing an absent override is idempotent")
}

func TestRoles_ClearPlatformRoleIsIdempotentAndAudited(t *testing.T) {
	logs := &bytes.Buffer{}
	router, fixture := newProviderRouter(t, logs)
	provider := seedProvider(t, fixture)
	fast := seedChatModel(t, fixture, provider, "openai/gpt-4.1-nano", llmmodel.CapabilityChat)
	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatFast, fast))

	require.Equal(t, http.StatusNoContent, send(router, http.MethodDelete, "/api/v1/model-roles/chat.fast", "", nil).Code)
	require.Empty(t, listOf(t, send(router, http.MethodGet, "/api/v1/model-roles", "", nil)))
	require.Equal(t, http.StatusNoContent, send(router, http.MethodDelete, "/api/v1/model-roles/chat.fast", "", nil).Code, "idempotent")
	require.Equal(t, http.StatusBadRequest, send(router, http.MethodDelete, "/api/v1/model-roles/not-a-role", "", nil).Code)
	var count int
	require.NoError(t, fixture.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE action = 'model_role.clear' AND target_id = 'chat.fast'`).Scan(&count))
	require.Equal(t, 2, count, "each clear is audited, like each assign")
	_, err := newResolveService(fixture).Resolve(platformContext(context.Background()), llmmodel.RoleChatFast)
	require.Equal(t, errors.CodeLLMRoleNotAssigned, errors.CodeOf(err))
}
