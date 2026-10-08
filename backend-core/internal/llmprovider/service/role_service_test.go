package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

func TestAssignRoleRejectsMissingCapability(t *testing.T) {
	s := newTestService(t, &fakeCatalog{})
	ctx := platformContext(context.Background())
	p := seedProvider(t, s)
	m, err := s.CreateModel(ctx, model.LLMModelInput{ProviderID: p.ID, ModelRef: "x/chat-only", DisplayName: "Chat only", Capabilities: []model.Capability{model.CapabilityChat}})
	require.NoError(t, err)
	require.Equal(t, errors.CodeLLMModelCapabilityMismatch, code(t, s.AssignRole(ctx, model.RoleChatDefault, m.ID)))
	require.NoError(t, s.AssignRole(ctx, model.RoleChatFast, m.ID))
}
