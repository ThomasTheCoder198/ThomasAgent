package model

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
