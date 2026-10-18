package integration

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	llmrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

const (
	lockWaitTimeout  = 10 * time.Second
	lockWaitInterval = 20 * time.Millisecond
)

// waitForLockWaiter blocks until some backend of the test database is waiting on a row lock, which is how the test
// knows the goroutine under test has reached its conflicting statement before it lets the other transaction go on.
func waitForLockWaiter(t *testing.T, fixture *llmFixture) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting int
		require.NoError(t, fixture.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting))
		return waiting > 0
	}, lockWaitTimeout, lockWaitInterval, "no transaction is waiting on a lock")
}

// requireRoleStillFits fails when the role points at a model that lacks what the role needs (A2-5).
func requireRoleStillFits(t *testing.T, fixture *llmFixture, models llmrepository.LLMModelRepo, roles llmrepository.RoleRepo, role llmmodel.Role) {
	t.Helper()
	ctx := platformContext(context.Background())
	modelID, assigned, err := roles.RoleModelID(ctx, fixture.pool, role)
	require.NoError(t, err)
	if !assigned {
		return
	}
	current, err := models.GetModel(ctx, fixture.pool, modelID)
	require.NoError(t, err)
	require.True(t, current.FitsRole(role), "role %s points at a model with capabilities %v", role, current.Capabilities)
}

func TestModels_EditedCapabilitiesSurviveSync(t *testing.T) {
	logs := &bytes.Buffer{}
	router, fixture := newProviderRouter(t, logs)
	provider := seedProvider(t, fixture)
	fixture.catalog.models = []llmmodel.RemoteModel{{ModelRef: "google/gemini-2.5-flash", DisplayName: "Gemini", Capabilities: []llmmodel.Capability{llmmodel.CapabilityChat}}}
	require.Equal(t, http.StatusOK, send(router, http.MethodPost, "/api/v1/providers/"+provider.ID.String()+"/sync", "", nil).Code)
	synced := listOf(t, send(router, http.MethodGet, "/api/v1/models", "", nil))[0].(map[string]any)
	id := synced["id"].(string)
	require.Equal(t, false, synced["capabilitiesEdited"])

	res := send(router, http.MethodPatch, "/api/v1/models/"+id, `{"capabilities":["chat","tools","chat"]}`, nil)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	edited := dataOf(t, res)
	require.Equal(t, []any{"chat", "tools"}, edited["capabilities"])
	require.Equal(t, true, edited["capabilitiesEdited"])

	require.Equal(t, http.StatusOK, send(router, http.MethodPost, "/api/v1/providers/"+provider.ID.String()+"/sync", "", nil).Code)
	after := listOf(t, send(router, http.MethodGet, "/api/v1/models", "", nil))[0].(map[string]any)
	require.Equal(t, []any{"chat", "tools"}, after["capabilities"], "a sync keeps the operator's edit")
}

func TestModels_CapabilityEditIsValidatedAndGuardsAssignedRoles(t *testing.T) {
	logs := &bytes.Buffer{}
	router, fixture := newProviderRouter(t, logs)
	provider := seedProvider(t, fixture)
	model := seedChatModel(t, fixture, provider, "x/tools", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	path := "/api/v1/models/" + model.String()

	require.Equal(t, http.StatusBadRequest, send(router, http.MethodPatch, path, `{"capabilities":[]}`, nil).Code)
	require.Equal(t, http.StatusBadRequest, send(router, http.MethodPatch, path, `{"capabilities":["telepathy"]}`, nil).Code)
	require.Equal(t, http.StatusBadRequest, send(router, http.MethodPatch, path, `{"capabilities":["embedding"]}`, nil).Code, "embedding needs dims")

	require.NoError(t, fixture.roles.AssignRole(platformContext(context.Background()), llmmodel.RoleChatDefault, model))
	res := send(router, http.MethodPatch, path, `{"capabilities":["chat"]}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.Code)
	require.Equal(t, "LLM_MODEL_CAPABILITY_MISMATCH", errorOf(t, res)["code"], "chat.default still needs tools")
	require.Equal(t, http.StatusOK, send(router, http.MethodPatch, path, `{"capabilities":["chat","tools","vision"]}`, nil).Code)
	require.Equal(t, http.StatusNotFound, send(router, http.MethodPatch, "/api/v1/models/"+provider.ID.String(), `{"capabilities":["chat"]}`, nil).Code)
}

// An assignment that already read the model's old capabilities and a capability edit that reads the roles before the
// assignment commits must not both succeed: the edit locks the model row first, so it waits for the assignment and then
// sees the new role.
func TestModels_CapabilityEditWaitsForAConcurrentAssignment(t *testing.T) {
	fixture := newLLMFixture(t)
	provider := seedProvider(t, fixture)
	modelID := seedChatModel(t, fixture, provider, "x/tools", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	models := llmrepository.NewLLMModelRepo(llmrepository.NewProviderRepo())
	roles := llmrepository.NewRoleRepo()
	ctx := platformContext(context.Background())

	assignment, err := fixture.pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = assignment.Rollback(ctx) }()
	current, err := models.GetModelForShare(ctx, assignment, modelID)
	require.NoError(t, err)
	require.True(t, current.FitsRole(llmmodel.RoleChatDefault), "the assignment sees the old capabilities and accepts them")

	edit := make(chan error, 1)
	go func() {
		_, err := fixture.models.UpdateCapabilities(ctx, modelID, []llmmodel.Capability{llmmodel.CapabilityChat})
		edit <- err
	}()
	waitForLockWaiter(t, fixture)

	require.NoError(t, roles.UpsertRole(ctx, assignment, llmmodel.RoleChatDefault, modelID))
	require.NoError(t, assignment.Commit(ctx))

	select {
	case err := <-edit:
		require.Error(t, err)
		require.Equal(t, errors.CodeLLMModelCapabilityMismatch, errors.ToAppError(err).Code, "the edit must see the committed assignment and be refused")
	case <-time.After(lockWaitTimeout):
		t.Fatal("the capability edit never finished")
	}
	requireRoleStillFits(t, fixture, models, roles, llmmodel.RoleChatDefault)
}

// The other order: an edit that already holds the model row makes a concurrent assignment wait, and the assignment then
// reads the edited capabilities and is refused.
func TestModels_AssignmentWaitsForAConcurrentCapabilityEdit(t *testing.T) {
	fixture := newLLMFixture(t)
	provider := seedProvider(t, fixture)
	modelID := seedChatModel(t, fixture, provider, "x/tools", llmmodel.CapabilityChat, llmmodel.CapabilityTools)
	models := llmrepository.NewLLMModelRepo(llmrepository.NewProviderRepo())
	roles := llmrepository.NewRoleRepo()
	ctx := platformContext(context.Background())

	edit, err := fixture.pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = edit.Rollback(ctx) }()
	_, err = models.GetModelForUpdate(ctx, edit, modelID)
	require.NoError(t, err)
	require.Empty(t, mustRolesUsingModel(t, roles, edit, modelID))

	assignment := make(chan error, 1)
	go func() { assignment <- fixture.roles.AssignRole(ctx, llmmodel.RoleChatDefault, modelID) }()
	waitForLockWaiter(t, fixture)

	_, err = models.UpdateCapabilities(ctx, edit, modelID, []llmmodel.Capability{llmmodel.CapabilityChat})
	require.NoError(t, err)
	require.NoError(t, edit.Commit(ctx))

	select {
	case err := <-assignment:
		require.Error(t, err)
		require.Equal(t, errors.CodeLLMModelCapabilityMismatch, errors.ToAppError(err).Code, "the assignment must see the edited capabilities")
	case <-time.After(lockWaitTimeout):
		t.Fatal("the role assignment never finished")
	}
	requireRoleStillFits(t, fixture, models, roles, llmmodel.RoleChatDefault)
}

func mustRolesUsingModel(t *testing.T, roles llmrepository.RoleRepo, db postgres.DBTX, modelID uuid.UUID) []llmmodel.Role {
	t.Helper()
	using, err := roles.RolesUsingModel(context.Background(), db, modelID)
	require.NoError(t, err)
	return using
}
