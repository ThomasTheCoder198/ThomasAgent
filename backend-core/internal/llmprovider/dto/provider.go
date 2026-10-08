package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

// ProviderRequest is write-only for the API key: responses never echo it.
type ProviderRequest struct {
	Kind    model.ProviderKind `json:"kind"`
	Name    string             `json:"name"`
	BaseURL string             `json:"baseUrl"`
	APIKey  *string            `json:"apiKey"`
	Enabled *bool              `json:"enabled"`
}

func (request ProviderRequest) ToInput() model.ProviderInput { return model.ProviderInput(request) }

type ProviderResponse struct {
	ID        uuid.UUID          `json:"id"`
	Kind      model.ProviderKind `json:"kind"`
	Name      string             `json:"name"`
	BaseURL   string             `json:"baseUrl"`
	HasAPIKey bool               `json:"hasApiKey"`
	Enabled   bool               `json:"enabled"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

func NewProviderResponse(provider model.Provider) ProviderResponse {
	return ProviderResponse(provider)
}

func NewProviderResponses(providers []model.Provider) []ProviderResponse {
	responses := make([]ProviderResponse, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, NewProviderResponse(provider))
	}
	return responses
}

type TestProviderResponse struct {
	OK         bool `json:"ok"`
	ModelCount int  `json:"modelCount"`
}

type SyncResultResponse struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func NewSyncResultResponse(result model.SyncResult) SyncResultResponse {
	return SyncResultResponse(result)
}
