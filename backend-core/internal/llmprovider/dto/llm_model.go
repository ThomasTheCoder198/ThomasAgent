package dto

import (
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

type LLMModelRequest struct {
	ProviderID         uuid.UUID          `json:"providerId"`
	ModelRef           string             `json:"modelRef"`
	DisplayName        string             `json:"displayName"`
	Capabilities       []model.Capability `json:"capabilities"`
	ContextWindow      *int               `json:"contextWindow"`
	EmbeddingDims      *int               `json:"embeddingDims"`
	InputPricePerMTok  *float64           `json:"inputPricePerMTok"`
	OutputPricePerMTok *float64           `json:"outputPricePerMTok"`
}

func (request LLMModelRequest) ToInput() model.LLMModelInput { return model.LLMModelInput(request) }

type LLMModelResponse struct {
	ID                 uuid.UUID          `json:"id"`
	ProviderID         uuid.UUID          `json:"providerId"`
	ModelRef           string             `json:"modelRef"`
	DisplayName        string             `json:"displayName"`
	Capabilities       []model.Capability `json:"capabilities"`
	ContextWindow      *int               `json:"contextWindow,omitempty"`
	EmbeddingDims      *int               `json:"embeddingDims,omitempty"`
	InputPricePerMTok  *float64           `json:"inputPricePerMTok,omitempty"`
	OutputPricePerMTok *float64           `json:"outputPricePerMTok,omitempty"`
	Source             string             `json:"source"`
}

func NewLLMModelResponse(llmModel model.LLMModel) LLMModelResponse {
	return LLMModelResponse(llmModel)
}

func NewLLMModelResponses(llmModels []model.LLMModel) []LLMModelResponse {
	responses := make([]LLMModelResponse, 0, len(llmModels))
	for _, llmModel := range llmModels {
		responses = append(responses, NewLLMModelResponse(llmModel))
	}
	return responses
}
