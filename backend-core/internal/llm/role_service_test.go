package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestAssignRoleRejectsMissingCapability(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/chat-only", DisplayName: "Chat only", Capabilities: []Capability{CapabilityChat}})
	require.NoError(t, err)
	require.Equal(t, errors.CodeLLMModelCapabilityMismatch, code(t, s.AssignRole(ctx, RoleChatDefault, m.ID)))
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))
}

func TestResolveReturnsDecryptedKeyAndFallsBack(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, _ := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, ModelRef: "x/fast", DisplayName: "Fast", Capabilities: []Capability{CapabilityChat}})
	require.NoError(t, s.AssignRole(ctx, RoleChatFast, m.ID))

	got, err := s.Resolve(ctx, RoleChatFast)
	require.NoError(t, err)
	require.Equal(t, secretKey, got.APIKey)
	require.Equal(t, "x/fast", got.ModelRef)

	got, err = s.Resolve(ctx, RoleDecision)
	require.NoError(t, err)
	require.Equal(t, RoleChatFast, got.Role)
	require.Equal(t, RoleDecision, got.FallbackFrom)

	_, err = s.Resolve(ctx, RoleEmbedding)
	require.Equal(t, errors.CodeLLMRoleNotAssigned, code(t, err))
}

func TestResolveDisabledProviderIsUnavailable(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	p := seedProvider(t, s)
	m, err := s.CreateModel(platformContext(context.Background()), ModelInput{ProviderID: p.ID, ModelRef: "x", Capabilities: []Capability{CapabilityChat}})
	require.NoError(t, err)
	require.NoError(t, s.AssignRole(platformContext(context.Background()), RoleChatFast, m.ID))
	_, err = s.UpdateProvider(platformContext(context.Background()), p.ID, ProviderInput{Enabled: ptr(false)})
	require.NoError(t, err)
	_, err = s.Resolve(platformContext(context.Background()), RoleChatFast)
	require.Equal(t, errors.CodeProviderUnavailable, code(t, err))
}
