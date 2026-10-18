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

// Where a resolved model came from (index ResolvedModel.Source); None is used by the org role list only.
const (
	ResolvedSourceOrg      = "org"
	ResolvedSourcePlatform = "platform"
	ResolvedSourceFallback = "fallback"
	ResolvedSourceNone     = "none"
)

// CoreChatRoles are the roles core's own LLM calls use, in Settings order; embedding and rerank belong to ragx (O6).
var CoreChatRoles = []Role{RoleChatDefault, RoleChatFast, RoleVision, RoleDecision}

// ResolvedModel is everything an internal caller needs to call the model behind a role, including the decrypted
// API key. FallbackFrom is set when an unassigned role fell back. KeyVersion changes when the key is replaced
// ("" for a keyless provider) so clients cached per key can be refreshed without ever keying on the key.
type ResolvedModel struct {
	Role          Role
	ProviderID    uuid.UUID
	ProviderKind  ProviderKind
	BaseURL       string
	APIKey        string
	KeyVersion    string
	ModelRef      string
	Capabilities  []Capability
	ContextWindow *int
	EmbeddingDims *int
	FallbackFrom  Role
	Source        string
}

// OrgRoleView is one row of GET /orgs/{orgId}/model-roles (ruling A2-4).
type OrgRoleView struct {
	Role             Role
	ModelID          *uuid.UUID
	EffectiveModelID *uuid.UUID
	Source           string
}
