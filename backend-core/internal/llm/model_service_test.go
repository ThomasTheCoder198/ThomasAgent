package llm

import (
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func TestSyncModelsUpsertsFromCatalog(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{
		{ModelRef: "anthropic/claude-sonnet-5", DisplayName: "Claude Sonnet 5", Capabilities: []Capability{CapabilityChat, CapabilityTools, CapabilityVision}},
		{ModelRef: "openai/gpt-5-mini", DisplayName: "GPT-5 mini", Capabilities: []Capability{CapabilityChat}},
	}}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)

	res, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, SyncResult{Added: 2}, res)
	res, err = s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, SyncResult{Updated: 2}, res)

	vision := CapabilityVision
	models, err := s.ListModels(ctx, nil, &vision)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "anthropic/claude-sonnet-5", models[0].ModelRef)
}

func TestSyncSkipsInvalidAndManualCollisionsAndRemovesUnreferencedStaleRows(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "manual", Capabilities: []Capability{CapabilityChat}}, {ModelRef: "keep", Capabilities: []Capability{CapabilityChat}}}}
	s := newTestService(t, cat)
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	manual, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "manual", Capabilities: []Capability{CapabilityVision}, EmbeddingDims: ptr(384)})
	require.NoError(t, err)
	first, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 1, first.Skipped)
	_, err = s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "keep", Capabilities: []Capability{CapabilityVision}})
	require.Equal(t, errors.CodeLLMNameTaken, code(t, err))
	modelsBefore, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	var keepID uuid.UUID
	for _, model := range modelsBefore {
		if model.ModelRef == "keep" {
			keepID = model.ID
		}
	}
	require.NotEqual(t, uuid.Nil, keepID)
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, keepID))
	cat.models = []RemoteModel{{ModelRef: "new", Capabilities: []Capability{CapabilityChat}}, {ModelRef: "bad", Capabilities: []Capability{"invalid"}}}
	_, err = s.SyncModels(ctx, p.ID)
	require.Equal(t, errors.CodeLLMProviderRejected, code(t, err))
	unchanged, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	require.Equal(t, modelsBefore, unchanged, "malformed snapshots preserve existing models and roll back valid siblings")
	cat.models = []RemoteModel{{ModelRef: "new", Capabilities: []Capability{CapabilityChat}}}
	second, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 1, second.Added)
	models, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 3, "a stale sync row referenced by a role remains available")
	for _, model := range models {
		if model.ID == manual.ID {
			require.Equal(t, []Capability{CapabilityVision}, model.Capabilities)
			require.Equal(t, ptr(384), model.EmbeddingDims)
		}
	}
}

func TestSyncUnsupportedKind(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p, err := s.CreateProvider(platformContext(context.Background()), ProviderInput{Kind: KindCohere, Name: "Cohere"})
	require.NoError(t, err)
	_, err = s.SyncModels(platformContext(context.Background()), p.ID)
	require.Equal(t, errors.CodeLLMProviderSyncUnsupported, code(t, err))
}

func TestDeleteModelInUseIsConflict(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapabilityChat}})
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))
	require.Equal(t, errors.CodeLLMModelInUse, code(t, s.DeleteModel(ctx, m.ID)))
	require.Equal(t, errors.CodeLLMModelInUse, code(t, s.DeleteProvider(ctx, p.ID)))
}

func TestCreateModelValidatesCapabilities(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	_, err := s.CreateModel(platformContext(context.Background()), ModelInput{ProviderID: p.ID, ModelRef: "x", DisplayName: "x", Capabilities: []Capability{"telepathy"}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
	_, err = s.CreateModel(platformContext(context.Background()), ModelInput{ProviderID: p.ID, ModelRef: "e", DisplayName: "e", Capabilities: []Capability{CapabilityEmbedding}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err), "embedding models need embeddingDims")
}

func TestCreateModelRejectsCollisionAndPreservesOriginal(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	provider := seedProvider(t, s)
	original, err := s.CreateModel(platformContext(t.Context()), ModelInput{ProviderID: provider.ID, ModelRef: "manual", Capabilities: []Capability{CapabilityEmbedding}, EmbeddingDims: ptr(384)})
	require.NoError(t, err)
	_, err = s.CreateModel(platformContext(t.Context()), ModelInput{ProviderID: provider.ID, ModelRef: "manual", Capabilities: []Capability{CapabilityChat}})
	require.Error(t, err)
	require.Equal(t, errors.CodeLLMNameTaken, errors.ToAppError(err).Code)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, original, models[0])
}

func TestSyncRollsBackMalformedRemoteModel(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "seed", Capabilities: []Capability{CapabilityChat}}}}
	s := newTestService(t, cat)
	ctx := platformContext(t.Context())
	p := seedProvider(t, s)
	_, err := s.SyncModels(ctx, p.ID)
	require.NoError(t, err)
	before, err := s.ListModels(ctx, &p.ID, nil)
	require.NoError(t, err)
	var auditBefore int
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='model.sync'`).Scan(&auditBefore))
	cat.models = []RemoteModel{{ModelRef: "valid", Capabilities: []Capability{CapabilityChat}}, {ModelRef: " ", Capabilities: []Capability{CapabilityChat}}}
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
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "seed"}}}
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
	other, err := s.CreateProvider(tenant.WithID(t.Context(), "other"), ProviderInput{Kind: KindOpenRouter, Name: "OR"})
	require.NoError(t, err, "same provider name can exist in different context tenants")
	providers, err := s.ListProviders(platformContext(t.Context()))
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, provider.ID, providers[0].ID)
	require.NotEqual(t, other.ID, providers[0].ID)
}

func TestSyncKeepsStaleModelsReferencedByAnotherTenantRole(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "shared", Capabilities: []Capability{CapabilityChat}}}}
	s := newTestService(t, cat)
	provider := seedProvider(t, s)
	_, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	_, err = s.pool.Exec(platformContext(t.Context()), `INSERT INTO model_roles (tenant_id,role,model_id) VALUES ('other','chat.fast',$1)`, models[0].ID)
	require.NoError(t, err)
	cat.models = []RemoteModel{{ModelRef: "new"}}
	_, err = s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	models, err = s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 2, "a complete nonempty snapshot retains rows referenced by another tenant")
}

func TestSyncCapsRemoteCountWithoutDeletingRowsOutsidePartialSnapshot(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "last"}}}
	s := newTestService(t, cat)
	provider := seedProvider(t, s)
	_, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	cat.models = make([]RemoteModel, 1001)
	for i := range 1000 {
		cat.models[i] = RemoteModel{ModelRef: "model-" + strconv.Itoa(i)}
	}
	cat.models[1000] = RemoteModel{ModelRef: "last"}
	result, err := s.SyncModels(platformContext(t.Context()), provider.ID)
	require.NoError(t, err)
	require.Equal(t, 1000, result.Added)
	require.Equal(t, 1, result.Skipped)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 1001, len(models), "a capped remote snapshot cannot prove which existing rows are stale")
}

func TestModelRejectsInvalidNumericMetadata(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	for _, in := range []ModelInput{
		{ModelRef: "x", ContextWindow: ptr(-1)},
		{ModelRef: "x", EmbeddingDims: ptr(0)},
		{ModelRef: "x", InputPricePerMTok: ptr(-1.0)},
	} {
		_, err := s.CreateModel(platformContext(t.Context()), in)
		require.Equal(t, errors.CodeValidationFailed, code(t, err))
	}
}

func TestSyncSkipsAndCountsMalformedRemoteModel(t *testing.T) {
	cat := &fakeCatalog{models: []RemoteModel{{ModelRef: "valid", Capabilities: []Capability{CapabilityChat}}, {ModelRef: " ", Capabilities: []Capability{CapabilityChat}}}}
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
	require.Equal(t, SyncResult{Added: 1}, result)
	models, err = s.ListModels(platformContext(context.Background()), nil, nil)
	require.NoError(t, err)
	require.Len(t, models, 1)
}
