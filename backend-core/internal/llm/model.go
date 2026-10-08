package llm

import "github.com/google/uuid"

type Capability string

const (
	CapabilityChat      Capability = "chat"
	CapabilityTools     Capability = "tools"
	CapabilityVision    Capability = "vision"
	CapabilityReasoning Capability = "reasoning"
	CapabilityEmbedding Capability = "embedding"
	CapabilityRerank    Capability = "rerank"
	CapabilityDecision  Capability = "decision"
)

func IsValidCapability(capability Capability) bool {
	switch capability {
	case CapabilityChat, CapabilityTools, CapabilityVision, CapabilityReasoning, CapabilityEmbedding, CapabilityRerank, CapabilityDecision:
		return true
	}
	return false
}

// Model is one model a provider serves, added by hand or by catalog sync.
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

// RemoteModel is one entry of a provider's published model list, before it is stored.
type RemoteModel struct {
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}

type SyncResult struct {
	Added   int
	Updated int
	Skipped int
}
