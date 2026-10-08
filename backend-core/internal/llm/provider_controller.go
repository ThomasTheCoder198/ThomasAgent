package llm

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// ProviderRequest is write-only for the API key: responses never echo it.
type ProviderRequest struct {
	Kind    ProviderKind `json:"kind"`
	Name    string       `json:"name"`
	BaseURL string       `json:"baseUrl"`
	APIKey  *string      `json:"apiKey"`
	Enabled *bool        `json:"enabled"`
}

func (request ProviderRequest) toInput() ProviderInput { return ProviderInput(request) }

type ProviderResponse struct {
	ID        uuid.UUID    `json:"id"`
	Kind      ProviderKind `json:"kind"`
	Name      string       `json:"name"`
	BaseURL   string       `json:"baseUrl"`
	HasAPIKey bool         `json:"hasApiKey"`
	Enabled   bool         `json:"enabled"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

func newProviderResponse(provider Provider) ProviderResponse { return ProviderResponse(provider) }

func newProviderResponses(providers []Provider) []ProviderResponse {
	responses := make([]ProviderResponse, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, newProviderResponse(provider))
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

type providerOperations interface {
	CreateProvider(ctx context.Context, input ProviderInput) (Provider, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, input ProviderInput) (Provider, error)
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	ListProviders(ctx context.Context) ([]Provider, error)
	TestProvider(ctx context.Context, id uuid.UUID) (int, error)
	SyncModels(ctx context.Context, providerID uuid.UUID) (SyncResult, error)
}

// ProviderController serves /api/v1/providers.
type ProviderController struct {
	service      providerOperations
	maxBodyBytes int64
}

func NewProviderController(service providerOperations, maxBodyBytes int64) *ProviderController {
	return &ProviderController{service: service, maxBodyBytes: maxBodyBytes}
}

func (h *ProviderController) HandleListProviders(c *gin.Context) {
	providers, err := h.service.ListProviders(c.Request.Context())
	httpx.Respond(c, http.StatusOK, newProviderResponses(providers), err)
}

func (h *ProviderController) HandleCreateProvider(c *gin.Context) {
	var body ProviderRequest
	if err := httpx.DecodeJSON(c.Writer, c.Request, &body, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	provider, err := h.service.CreateProvider(c.Request.Context(), body.toInput())
	httpx.Respond(c, http.StatusCreated, newProviderResponse(provider), err)
}

func (h *ProviderController) HandleUpdateProvider(c *gin.Context) {
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var body ProviderRequest
	if err := httpx.DecodeJSON(c.Writer, c.Request, &body, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	provider, err := h.service.UpdateProvider(c.Request.Context(), id, body.toInput())
	httpx.Respond(c, http.StatusOK, newProviderResponse(provider), err)
}

func (h *ProviderController) HandleDeleteProvider(c *gin.Context) {
	id, err := pathUUID(c, "providerId")
	if err == nil {
		err = h.service.DeleteProvider(c.Request.Context(), id)
	}
	httpx.Respond(c, http.StatusOK, DeleteResponse{Deleted: true}, err)
}

func (h *ProviderController) HandleTestProvider(c *gin.Context) {
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	modelCount, err := h.service.TestProvider(c.Request.Context(), id)
	httpx.Respond(c, http.StatusOK, TestProviderResponse{OK: true, ModelCount: modelCount}, err)
}

func (h *ProviderController) HandleSyncProvider(c *gin.Context) {
	id, err := pathUUID(c, "providerId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	result, err := h.service.SyncModels(c.Request.Context(), id)
	httpx.Respond(c, http.StatusOK, SyncResultResponse(result), err)
}
