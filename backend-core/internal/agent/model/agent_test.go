package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

func TestNormalizeInput(t *testing.T) {
	in := AgentInput{Name: "  Hỗ trợ  ", KBID: uuid.New()}
	require.NoError(t, NormalizeInput(&in))
	require.Equal(t, "Hỗ trợ", in.Name)
	for name, bad := range map[string]AgentInput{
		"empty name":   {Name: " ", KBID: uuid.New()},
		"long name":    {Name: strings.Repeat("ạ", NameMaxRunes+1), KBID: uuid.New()},
		"long desc":    {Name: "a", Description: strings.Repeat("a", DescriptionMaxRunes+1), KBID: uuid.New()},
		"long instr":   {Name: "a", Instructions: strings.Repeat("a", InstructionsMaxRunes+1), KBID: uuid.New()},
		"missing kbId": {Name: "a"},
	} {
		t.Run(name, func(t *testing.T) { require.Equal(t, errors.CodeValidationFailed, errors.CodeOf(NormalizeInput(&bad))) })
	}
}

func TestAgentApply(t *testing.T) {
	chat, fast, other := uuid.New(), uuid.New(), uuid.New()
	kb, newKB := uuid.New(), uuid.New()
	name, desc, instr := "B", "d", "i"
	base := Agent{Name: "A", Description: "x", Instructions: "y", KBID: kb,
		ModelOverrides: map[llmmodel.Role]uuid.UUID{llmmodel.RoleChatDefault: chat, llmmodel.RoleChatFast: fast}}

	t.Run("empty patch keeps everything", func(t *testing.T) {
		in := base.Apply(AgentPatch{})
		require.Equal(t, AgentInput{Name: "A", Description: "x", Instructions: "y", KBID: kb, ModelOverrides: base.ModelOverrides}, in)
	})
	t.Run("scalar fields and the KB are replaced", func(t *testing.T) {
		in := base.Apply(AgentPatch{Name: &name, Description: &desc, Instructions: &instr, KBID: &newKB})
		require.Equal(t, AgentInput{Name: "B", Description: "d", Instructions: "i", KBID: newKB, ModelOverrides: base.ModelOverrides}, in)
	})
	t.Run("a nil override removes the role and a value sets it", func(t *testing.T) {
		in := base.Apply(AgentPatch{ModelOverrides: map[llmmodel.Role]*uuid.UUID{llmmodel.RoleChatDefault: nil, llmmodel.RoleVision: &other}})
		require.Equal(t, map[llmmodel.Role]uuid.UUID{llmmodel.RoleChatFast: fast, llmmodel.RoleVision: other}, in.ModelOverrides)
	})
	t.Run("the stored map is never mutated", func(t *testing.T) {
		base.Apply(AgentPatch{ModelOverrides: map[llmmodel.Role]*uuid.UUID{llmmodel.RoleChatDefault: nil}})
		require.Contains(t, base.ModelOverrides, llmmodel.RoleChatDefault)
	})
	t.Run("a nil stored map becomes an empty map", func(t *testing.T) {
		in := Agent{Name: "A", KBID: kb}.Apply(AgentPatch{ModelOverrides: map[llmmodel.Role]*uuid.UUID{llmmodel.RoleChatDefault: nil}})
		require.NotNil(t, in.ModelOverrides)
		require.Empty(t, in.ModelOverrides)
	})
}
