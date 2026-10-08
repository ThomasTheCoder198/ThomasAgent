package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

type ProviderRequest struct {
	Kind    model.ProviderKind `json:"kind"`
	Name    string             `json:"name"`
	BaseURL string             `json:"baseUrl"`
	APIKey  *string            `json:"apiKey"`
	Enabled *bool              `json:"enabled"`
}

func (request ProviderRequest) ToModel() model.ProviderInput {
	return model.ProviderInput{
		Kind: request.Kind, Name: request.Name, BaseURL: request.BaseURL,
		APIKey: request.APIKey, Enabled: request.Enabled,
	}
}

type ModelRequest struct {
	ProviderID         uuid.UUID          `json:"providerId"`
	ModelRef           string             `json:"modelRef"`
	DisplayName        string             `json:"displayName"`
	Capabilities       []model.Capability `json:"capabilities"`
	ContextWindow      *int               `json:"contextWindow"`
	EmbeddingDims      *int               `json:"embeddingDims"`
	InputPricePerMTok  *float64           `json:"inputPricePerMTok"`
	OutputPricePerMTok *float64           `json:"outputPricePerMTok"`
}

func (request ModelRequest) ToModel() model.ModelInput {
	return model.ModelInput{
		ProviderID: request.ProviderID, ModelRef: request.ModelRef,
		DisplayName: request.DisplayName, Capabilities: request.Capabilities,
		ContextWindow: request.ContextWindow, EmbeddingDims: request.EmbeddingDims,
		InputPricePerMTok: request.InputPricePerMTok, OutputPricePerMTok: request.OutputPricePerMTok,
	}
}

type AssignRoleRequest struct {
	ModelID uuid.UUID `json:"modelId"`
}

type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}

type TestProviderResponse struct {
	OK         bool `json:"ok"`
	ModelCount int  `json:"modelCount"`
}

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

func FromProvider(provider model.Provider) ProviderResponse {
	return ProviderResponse{
		ID: provider.ID, Kind: provider.Kind, Name: provider.Name, BaseURL: provider.BaseURL,
		HasAPIKey: provider.HasAPIKey, Enabled: provider.Enabled,
		CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
	}
}

func FromProviders(providers []model.Provider) []ProviderResponse {
	responses := make([]ProviderResponse, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, FromProvider(provider))
	}
	return responses
}

type ModelResponse struct {
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

func FromModel(entry model.Model) ModelResponse {
	return ModelResponse{
		ID: entry.ID, ProviderID: entry.ProviderID, ModelRef: entry.ModelRef,
		DisplayName: entry.DisplayName, Capabilities: entry.Capabilities,
		ContextWindow: entry.ContextWindow, EmbeddingDims: entry.EmbeddingDims,
		InputPricePerMTok: entry.InputPricePerMTok, OutputPricePerMTok: entry.OutputPricePerMTok,
		Source: entry.Source,
	}
}

func FromModels(entries []model.Model) []ModelResponse {
	responses := make([]ModelResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, FromModel(entry))
	}
	return responses
}

type RoleAssignmentResponse struct {
	Role    model.Role `json:"role"`
	ModelID uuid.UUID  `json:"modelId"`
}

type SyncResultResponse struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func FromSyncResult(result model.SyncResult) SyncResultResponse {
	return SyncResultResponse{Added: result.Added, Updated: result.Updated, Skipped: result.Skipped}
}

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

func FromResolvedModel(entry model.ResolvedModel) ResolvedModelResponse {
	return ResolvedModelResponse{
		Role: entry.Role, ProviderKind: entry.ProviderKind, BaseURL: entry.BaseURL,
		APIKey: entry.APIKey, ModelRef: entry.ModelRef, Capabilities: entry.Capabilities,
		EmbeddingDims: entry.EmbeddingDims, FallbackFrom: entry.FallbackFrom,
	}
}

func FromRoles(roles []model.RoleAssignment) []RoleAssignmentResponse {
	responses := make([]RoleAssignmentResponse, 0, len(roles))
	for _, role := range roles {
		responses = append(responses, RoleAssignmentResponse{Role: role.Role, ModelID: role.ModelID})
	}
	return responses
}
