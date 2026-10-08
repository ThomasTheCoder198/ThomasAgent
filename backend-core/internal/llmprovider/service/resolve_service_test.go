package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

func TestResolveReturnsDecryptedKeyAndFallsBack(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, model.LLMModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []model.Capability{model.CapabilityChat}})
	require.NoError(t, s.AssignRole(ctx, model.RoleChatFast, m.ID))

	got, err := s.Resolve(ctx, model.RoleChatFast)
	require.NoError(t, err)
	require.Equal(t, secretKey, got.APIKey)
	require.Equal(t, "x/fast", got.ModelRef)

	got, err = s.Resolve(ctx, model.RoleDecision)
	require.NoError(t, err)
	require.Equal(t, model.RoleChatFast, got.Role)
	require.Equal(t, model.RoleDecision, got.FallbackFrom)

	_, err = s.Resolve(ctx, model.RoleEmbedding)
	require.Equal(t, errors.CodeLLMRoleNotAssigned, code(t, err))
}

func TestResolveDisabledProviderIsUnavailable(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	m, err := s.CreateModel(platformContext(context.Background()), model.LLMModelInput{ProviderID: p.ID, ModelRef: "x", Capabilities: []model.Capability{model.CapabilityChat}})
	require.NoError(t, err)
	require.NoError(t, s.AssignRole(platformContext(context.Background()), model.RoleChatFast, m.ID))
	_, err = s.UpdateProvider(platformContext(context.Background()), p.ID, model.ProviderInput{Enabled: ptr(false)})
	require.NoError(t, err)
	_, err = s.Resolve(platformContext(context.Background()), model.RoleChatFast)
	require.Equal(t, errors.CodeProviderUnavailable, code(t, err))
}
