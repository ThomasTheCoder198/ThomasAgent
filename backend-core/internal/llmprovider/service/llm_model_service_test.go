package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

func TestDeleteModelInUseIsConflict(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, model.LLMModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []model.Capability{model.CapabilityChat}})
	require.NoError(t, s.AssignRole(ctx, model.RoleChatFast, m.ID))
	require.Equal(t, errors.CodeLLMModelInUse, code(t, s.DeleteModel(ctx, m.ID)))
	require.Equal(t, errors.CodeLLMModelInUse, code(t, s.DeleteProvider(ctx, p.ID)))
}

func TestCreateModelValidatesCapabilities(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	_, err := s.CreateModel(platformContext(context.Background()), model.LLMModelInput{ProviderID: p.ID, ModelRef: "x", DisplayName: "x", Capabilities: []model.Capability{"telepathy"}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err))
	_, err = s.CreateModel(platformContext(context.Background()), model.LLMModelInput{ProviderID: p.ID, ModelRef: "e", DisplayName: "e", Capabilities: []model.Capability{model.CapabilityEmbedding}})
	require.Equal(t, errors.CodeValidationFailed, code(t, err), "embedding models need embeddingDims")
}

func TestCreateModelRejectsCollisionAndPreservesOriginal(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	provider := seedProvider(t, s)
	original, err := s.CreateModel(platformContext(t.Context()), model.LLMModelInput{ProviderID: provider.ID, ModelRef: "manual", Capabilities: []model.Capability{model.CapabilityEmbedding}, EmbeddingDims: ptr(384)})
	require.NoError(t, err)
	_, err = s.CreateModel(platformContext(t.Context()), model.LLMModelInput{ProviderID: provider.ID, ModelRef: "manual", Capabilities: []model.Capability{model.CapabilityChat}})
	require.Error(t, err)
	require.Equal(t, errors.CodeLLMNameTaken, errors.ToAppError(err).Code)
	models, err := s.ListModels(platformContext(t.Context()), &provider.ID, nil)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, original, models[0])
}

func TestModelRejectsInvalidNumericMetadata(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	for _, in := range []model.LLMModelInput{
		{ModelRef: "x", ContextWindow: ptr(-1)},
		{ModelRef: "x", EmbeddingDims: ptr(0)},
		{ModelRef: "x", InputPricePerMTok: ptr(-1.0)},
	} {
		_, err := s.CreateModel(platformContext(t.Context()), in)
		require.Equal(t, errors.CodeValidationFailed, code(t, err))
	}
}
