package registry

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

func validKind(k ProviderKind) bool {
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

func validCapability(c Capability) bool {
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
func requirementsForRole(role Role) [][]Capability {
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
	ID        uuid.UUID    `json:"id"`
	Kind      ProviderKind `json:"kind"`
	Name      string       `json:"name"`
	BaseURL   string       `json:"baseUrl"`
	HasAPIKey bool         `json:"hasApiKey"`
	Enabled   bool         `json:"enabled"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type ProviderInput struct {
	Kind    ProviderKind `json:"kind"`
	Name    string       `json:"name"`
	BaseURL string       `json:"baseUrl"`
	APIKey  *string      `json:"apiKey"`
	Enabled *bool        `json:"enabled"`
}

type Model struct {
	ID                 uuid.UUID    `json:"id"`
	ProviderID         uuid.UUID    `json:"providerId"`
	ModelRef           string       `json:"modelRef"`
	DisplayName        string       `json:"displayName"`
	Capabilities       []Capability `json:"capabilities"`
	ContextWindow      *int         `json:"contextWindow,omitempty"`
	EmbeddingDims      *int         `json:"embeddingDims,omitempty"`
	InputPricePerMTok  *float64     `json:"inputPricePerMTok,omitempty"`
	OutputPricePerMTok *float64     `json:"outputPricePerMTok,omitempty"`
	Source             string       `json:"source"`
}

type ModelInput struct {
	ProviderID         uuid.UUID    `json:"providerId"`
	ModelRef           string       `json:"modelRef"`
	DisplayName        string       `json:"displayName"`
	Capabilities       []Capability `json:"capabilities"`
	ContextWindow      *int         `json:"contextWindow"`
	EmbeddingDims      *int         `json:"embeddingDims"`
	InputPricePerMTok  *float64     `json:"inputPricePerMTok"`
	OutputPricePerMTok *float64     `json:"outputPricePerMTok"`
}

type RoleAssignment struct {
	Role    Role      `json:"role"`
	ModelID uuid.UUID `json:"modelId"`
}

type ResolvedModel struct {
	Role          Role         `json:"role"`
	ProviderKind  ProviderKind `json:"providerKind"`
	BaseURL       string       `json:"baseUrl"`
	APIKey        string       `json:"apiKey"`
	ModelRef      string       `json:"modelRef"`
	Capabilities  []Capability `json:"capabilities"`
	EmbeddingDims *int         `json:"embeddingDims,omitempty"`
	FallbackFrom  Role         `json:"fallbackFrom,omitempty"`
}
