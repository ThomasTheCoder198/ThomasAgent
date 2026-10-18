package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestSyncModelsUpsertsFromCatalog(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{
		{ModelRef: "anthropic/claude-sonnet-5", DisplayName: "Claude Sonnet 5", Capabilities: []model.Capability{model.CapabilityChat, model.CapabilityTools, model.CapabilityVision}},
		{ModelRef: "openai/gpt-5-mini", DisplayName: "GPT-5 mini", Capabilities: []model.Capability{model.CapabilityChat}},
	}}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)

	res, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, model.SyncResult{Added: 2}, res)
	res, err = s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, model.SyncResult{Updated: 2}, res)

	vision := model.CapabilityVision
	models, err := s.ListModels(ctx, nil, &vision)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "anthropic/claude-sonnet-5", models[0].ModelRef)
}

func TestSyncSkipsInvalidAndManualCollisionsAndRemovesUnreferencedStaleRows(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "manual", Capabilities: []model.Capability{model.CapabilityChat}}, {ModelRef: "keep", Capabilities: []model.Capability{model.CapabilityChat}}}}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	manual, err := s.CreateModel(ctx, model.LLMModelInput{ProviderID: p.ID, ModelRef: "manual", Capabilities: []model.Capability{model.CapabilityVision}, EmbeddingDims: ptr(384)})
	require.NoError(t, err)
	first, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 1, first.Skipped)
	_, err = s.CreateModel(ctx, model.LLMModelInput{ProviderID: p.ID, ModelRef: "keep", Capabilities: []model.Capability{model.CapabilityVision}})
	require.Equal(t, errors.CodeLLMNameTaken, code(t, err))
	modelsBefore, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	var keepID uuid.UUID
	for _, llmModel := range modelsBefore {
		if llmModel.ModelRef == "keep" {
			keepID = llmModel.ID
		}
	}
	require.NotEqual(t, uuid.Nil, keepID)
	require.NoError(t, s.AssignRole(ctx, model.RoleChatFast, keepID))
	cat.models = []model.RemoteModel{{ModelRef: "new", Capabilities: []model.Capability{model.CapabilityChat}}, {ModelRef: "bad", Capabilities: []model.Capability{"invalid"}}}
	_, err = s.SyncModels(ctx, p.ID)
	require.Equal(t, errors.CodeLLMProviderRejected, code(t, err))
	unchanged, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	require.Equal(t, modelsBefore, unchanged, "malformed snapshots preserve existing models and roll back valid siblings")
	cat.models = []model.RemoteModel{{ModelRef: "new", Capabilities: []model.Capability{model.CapabilityChat}}}
	second, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 1, second.Added)
	models, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 3, "a stale sync row referenced by a role remains available")
	for _, llmModel := range models {
		if llmModel.ID == manual.ID {
			require.Equal(t, []model.Capability{model.CapabilityVision}, llmModel.Capabilities)
			require.Equal(t, ptr(384), llmModel.EmbeddingDims)
		}
	}
}

func TestSyncUnsupportedKind(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p, err := s.CreateProvider(platformContext(context.Background()), model.ProviderInput{Kind: model.KindCohere, Name: "Cohere"})
	require.NoError(t, err)
	_, err = s.SyncModels(platformContext(context.Background()), p.ID)
	require.Equal(t, errors.CodeLLMProviderSyncUnsupported, code(t, err))
}

func TestSyncRollsBackMalformedRemoteModel(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "seed", Capabilities: []model.Capability{model.CapabilityChat}}}}
	s := newTestService(t, cat)
	ctx := platformContext(t.Context())
	p := seedProvider(t, s)
	_, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	before, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	var auditBefore int
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='model.sync'`).Scan(&auditBefore))
	cat.models = []model.RemoteModel{{ModelRef: "valid", Capabilities: []model.Capability{model.CapabilityChat}}, {ModelRef: " ", Capabilities: []model.Capability{model.CapabilityChat}}}
	_, err = s.SyncModels(ctx, p.ID)
	require.Error(t, err)
	models, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	require.Equal(t, before, models, "malformed batches cannot add models or prune previous rows")
	var auditAfter int
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='model.sync'`).Scan(&auditAfter))
	require.Equal(t, auditBefore, auditAfter)
}

func TestSyncEmptyAndFailedSnapshotsPreserveModels(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "seed"}}}
	s := newTestService(t, cat)
	ctx := platformContext(t.Context())
	p := seedProvider(t, s)
	_, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	before, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	for _, failure := range []error{nil, errors.ErrProviderUnavailable} {
		cat.models, cat.err = nil, failure
		_, err := s.SyncModels(ctx, p.ID)
		if failure == nil {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, failure)
		}
		after, err := s.ListModels(ctx, &p.ID, nil)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func TestSyncRejectsMissingTenantAndIsolatesContextTenant(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	_, err := s.ListProviders(t.Context())
	require.Error(t, err, "missing tenant cannot query the platform catalog")
	provider := seedProvider(t, s)
	_, err = s.TestProvider(tenant.WithID(t.Context(), "other"), provider.ID)
	require.Equal(t, errors.CodeLLMProviderNotFound, code(t, err))
	other, err := s.CreateProvider(tenant.WithID(t.Context(), "other"), model.ProviderInput{Kind: model.KindOpenRouter, Name: "OR"})
	require.NoError(t, err, "same provider name can exist in different context tenants")
	providers, err := s.ListProviders(platformContext(t.Context()))
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, provider.ID, providers[0].ID)
	require.NotEqual(t, other.ID, providers[0].ID)
}

func TestSyncKeepsStaleModelsReferencedByAnotherTenantRole(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "shared", Capabilities: []model.Capability{model.CapabilityChat}}}}
	s := newTestService(t, cat)
	provider := seedProvider(t, s)
	_, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	_, err = s.pool.Exec(platformContext(t.Context()), `INSERT INTO model_roles (tenant_id,role,model_id) VALUES ('other','chat.fast',$1)`, models[0].ID)
	require.NoError(t, err)
	cat.models = []model.RemoteModel{{ModelRef: "new"}}
	_, err = s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	models, err = s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 2, "a complete nonempty snapshot retains rows referenced by another tenant")
}

func TestSyncCapsRemoteCountWithoutDeletingRowsOutsidePartialSnapshot(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "last"}}}
	s := newTestService(t, cat)
	provider := seedProvider(t, s)
	_, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	cat.models = make([]model.RemoteModel, 1001)
	for i := range 1000 {
		cat.models[i] = model.RemoteModel{ModelRef: "model-" + strconv.Itoa(i)}
	}
	cat.models[1000] = model.RemoteModel{ModelRef: "last"}
	result, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	require.Equal(t, 1000, result.Added)
	require.Equal(t, 1, result.Skipped)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 1001, len(models), "a capped remote snapshot cannot prove which existing rows are stale")
}

func TestSyncSkipsAndCountsMalformedRemoteModel(t *testing.T) {
	cat := &fakeCatalog{models: []model.RemoteModel{{ModelRef: "valid", Capabilities: []model.Capability{model.CapabilityChat}}, {ModelRef: " ", Capabilities: []model.Capability{model.CapabilityChat}}}}
	s := newTestService(t, cat)
	p := seedProvider(t, s)
	_, err := s.SyncModels(platformContext(context.Background()), p.ID)
	require.Equal(t, errors.CodeLLMProviderRejected, code(t, err))
	models, err := s.ListModels(platformContext(context.Background()), nil, nil)
	require.NoError(t, err)
	require.Empty(t, models, "invalid entries reject the entire batch and roll back valid siblings")
	cat.models = cat.models[:1]
	result, err := s.SyncModels(platformContext(context.Background()), p.ID)
	require.NoError(t, err)
	require.Equal(t, model.SyncResult{Added: 1}, result)
	models, err = s.ListModels(platformContext(context.Background()), nil, nil)
	require.NoError(t, err)
	require.Len(t, models, 1)
}

// Ruling F2a3-1: core roles never use embeddings (ragx owns them, O6), so a Google sync keeps only models that
// generate content and never stores an embedding capability from Google's list, which carries no dimensions.
func TestSyncGoogleKeepsGenerativeModelsAndSkipsTheRest(t *testing.T) {
	window := 1048576
	cat := &fakeCatalog{models: []model.RemoteModel{
		{ModelRef: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", Capabilities: []model.Capability{model.CapabilityChat}, ContextWindow: &window},
		{ModelRef: "gemini-embedding-001", DisplayName: "Gemini Embedding", Capabilities: []model.Capability{model.CapabilityEmbedding}},
		{ModelRef: "models-both", DisplayName: "Both", Capabilities: []model.Capability{model.CapabilityChat, model.CapabilityEmbedding}},
		{ModelRef: "aqa", DisplayName: "AQA"},
	}}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	provider, err := s.CreateProvider(ctx, model.ProviderInput{Kind: model.KindGoogle, Name: "Google", APIKey: ptr(secretKey)})
	require.NoError(t, err)

	res, err := s.SyncModels(ctx, provider.ID)
	require.NoError(t, err, "an embedding model without dimensions never fails the sync")
	require.Equal(t, model.SyncResult{Added: 2, Skipped: 2}, res)

	models, err := s.ListModels(ctx, &provider.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 2)
	byRef := map[string]model.LLMModel{}
	for _, llmModel := range models {
		byRef[llmModel.ModelRef] = llmModel
	}
	require.Equal(t, []model.Capability{model.CapabilityChat}, byRef["gemini-2.5-flash"].Capabilities)
	require.Equal(t, window, *byRef["gemini-2.5-flash"].ContextWindow)
	require.Equal(t, []model.Capability{model.CapabilityChat}, byRef["models-both"].Capabilities, "no embedding capability from Google")
	require.NotContains(t, byRef, "gemini-embedding-001")
	require.NotContains(t, byRef, "aqa")
}
