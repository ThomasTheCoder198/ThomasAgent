package dto

import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"

// ResolvedModelResponse includes the decrypted API key; only the service-token route returns it.
type ResolvedModelResponse struct {
	Role          model.Role         `json:"role"`
	ProviderKind  model.ProviderKind `json:"providerKind"`
	BaseURL       string             `json:"baseUrl"`
	APIKey        string             `json:"apiKey"`
	ModelRef      string             `json:"modelRef"`
	Capabilities  []model.Capability `json:"capabilities"`
	EmbeddingDims *int               `json:"embeddingDims,omitempty"`
	FallbackFrom  model.Role         `json:"fallbackFrom,omitempty"`
}

func NewResolvedModelResponse(resolved model.ResolvedModel) ResolvedModelResponse {
	return ResolvedModelResponse(resolved)
}
