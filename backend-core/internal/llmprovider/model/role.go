package model

import "github.com/google/uuid"

// Role is the job core asks a model to do; the owner assigns one model per role.
type Role string

const (
	RoleChatDefault Role = "chat.default"
	RoleChatFast    Role = "chat.fast"
	RoleVision      Role = "vision"
	RoleEmbedding   Role = "embedding"
	RoleRerank      Role = "rerank"
	RoleDecision    Role = "decision"
)

// RequirementsForRole lists alternative capability sets; unknown roles return nil.
func RequirementsForRole(role Role) [][]Capability {
	switch role {
	case RoleChatDefault:
		return [][]Capability{{CapabilityChat, CapabilityTools}}
	case RoleChatFast:
		return [][]Capability{{CapabilityChat}}
	case RoleVision:
		return [][]Capability{{CapabilityVision}}
	case RoleEmbedding:
		return [][]Capability{{CapabilityEmbedding}}
	case RoleRerank:
		return [][]Capability{{CapabilityRerank}, {CapabilityDecision}}
	case RoleDecision:
		return [][]Capability{{CapabilityDecision}}
	default:
		return nil
	}
}

type RoleAssignment struct {
	Role    Role
	ModelID uuid.UUID
}

// ResolvedModel is everything an internal caller needs to call the model behind a role,
// including the decrypted API key. FallbackFrom is set when an unassigned role fell back.
type ResolvedModel struct {
	Role          Role
	ProviderKind  ProviderKind
	BaseURL       string
	APIKey        string
	ModelRef      string
	Capabilities  []Capability
	EmbeddingDims *int
	FallbackFrom  Role
}
