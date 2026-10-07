package model

import (
	"time"

	"github.com/google/uuid"
)

type ProviderKind string

const (
	KindOpenRouter       ProviderKind = "openrouter"
	KindOpenAI           ProviderKind = "openai"
	KindAnthropic        ProviderKind = "anthropic"
	KindGoogle           ProviderKind = "google"
	KindAzure            ProviderKind = "azure"
	KindOllama           ProviderKind = "ollama"
	KindOpenAICompatible ProviderKind = "openai_compatible"
	KindCohere           ProviderKind = "cohere"
	KindVoyage           ProviderKind = "voyage"
	KindJina             ProviderKind = "jina"
	KindTypeSafe         ProviderKind = "typesafe"
)

// Kinds without a default must be given a base URL by the owner.
func DefaultBaseURL(kind ProviderKind) string {
	switch kind {
	case KindOpenRouter:
		return "https://openrouter.ai/api/v1"
	case KindOpenAI:
		return "https://api.openai.com/v1"
	case KindAnthropic:
		return "https://api.anthropic.com/v1"
	case KindGoogle:
		return "https://generativelanguage.googleapis.com/v1beta"
	case KindCohere:
		return "https://api.cohere.com/v2"
	case KindVoyage:
		return "https://api.voyageai.com/v1"
	case KindJina:
		return "https://api.jina.ai/v1"
	case KindTypeSafe:
		return "https://api.typesafe.ai/v1"
	default:
		return ""
	}
}

func ValidKind(k ProviderKind) bool {
	switch k {
	case KindOpenRouter, KindOpenAI, KindAnthropic, KindGoogle, KindAzure, KindOllama, KindOpenAICompatible, KindCohere, KindVoyage, KindJina, KindTypeSafe:
		return true
	}
	return false
}

type Capability string

const (
	CapChat      Capability = "chat"
	CapTools     Capability = "tools"
	CapVision    Capability = "vision"
	CapReasoning Capability = "reasoning"
	CapEmbedding Capability = "embedding"
	CapRerank    Capability = "rerank"
	CapDecision  Capability = "decision"
)

func ValidCapability(c Capability) bool {
	switch c {
	case CapChat, CapTools, CapVision, CapReasoning, CapEmbedding, CapRerank, CapDecision:
		return true
	}
	return false
}

type Role string

const (
	RoleChatDefault Role = "chat.default"
	RoleChatFast    Role = "chat.fast"
	RoleVision      Role = "vision"
	RoleEmbedding   Role = "embedding"
	RoleRerank      Role = "rerank"
	RoleDecision    Role = "decision"
)

// Each role is satisfied by every capability in at least one set.
func RequirementsForRole(role Role) [][]Capability {
	switch role {
	case RoleChatDefault:
		return [][]Capability{{CapChat, CapTools}}
	case RoleChatFast:
		return [][]Capability{{CapChat}}
	case RoleVision:
		return [][]Capability{{CapVision}}
	case RoleEmbedding:
		return [][]Capability{{CapEmbedding}}
	case RoleRerank:
		return [][]Capability{{CapRerank}, {CapDecision}}
	case RoleDecision:
		return [][]Capability{{CapDecision}}
	default:
		return nil
	}
}

type Provider struct {
	ID        uuid.UUID
	Kind      ProviderKind
	Name      string
	BaseURL   string
	HasAPIKey bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProviderInput struct {
	Kind    ProviderKind
	Name    string
	BaseURL string
	APIKey  *string
	Enabled *bool
}

type Model struct {
	ID                 uuid.UUID
	ProviderID         uuid.UUID
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	EmbeddingDims      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
	Source             string
}

type ModelInput struct {
	ProviderID         uuid.UUID
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	EmbeddingDims      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}

type RoleAssignment struct {
	Role    Role
	ModelID uuid.UUID
}

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

type SyncResult struct {
	Added   int
	Updated int
	Skipped int
}

type RemoteModel struct {
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}
