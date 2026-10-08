package model

import "github.com/google/uuid"

// Where a model row came from; sync never overwrites a manual row.
const (
	ModelSourceManual = "manual"
	ModelSourceSync   = "sync"
)

// LLMModel is one model a provider serves, added by hand or by catalog sync.
type LLMModel struct {
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

type LLMModelInput struct {
	ProviderID         uuid.UUID
	ModelRef           string
	DisplayName        string
	Capabilities       []Capability
	ContextWindow      *int
	EmbeddingDims      *int
	InputPricePerMTok  *float64
	OutputPricePerMTok *float64
}

// FitsRole reports whether the model has every capability of at least one set the role accepts.
func (m LLMModel) FitsRole(role Role) bool {
	has := make(map[Capability]bool, len(m.Capabilities))
	for _, capability := range m.Capabilities {
		has[capability] = true
	}
	for _, required := range RequirementsForRole(role) {
		fits := true
		for _, capability := range required {
			fits = fits && has[capability]
		}
		if fits {
			return true
		}
	}
	return false
}
