package llm

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type ModelRequest struct {
	ProviderID         uuid.UUID    `json:"providerId"`
	ModelRef           string       `json:"modelRef"`
	DisplayName        string       `json:"displayName"`
	Capabilities       []Capability `json:"capabilities"`
	ContextWindow      *int         `json:"contextWindow"`
	EmbeddingDims      *int         `json:"embeddingDims"`
	InputPricePerMTok  *float64     `json:"inputPricePerMTok"`
	OutputPricePerMTok *float64     `json:"outputPricePerMTok"`
}

func (request ModelRequest) toInput() ModelInput { return ModelInput(request) }

type ModelResponse struct {
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

func newModelResponse(model Model) ModelResponse { return ModelResponse(model) }

func newModelResponses(models []Model) []ModelResponse {
	responses := make([]ModelResponse, 0, len(models))
	for _, model := range models {
		responses = append(responses, newModelResponse(model))
	}
	return responses
}

type modelOperations interface {
	CreateModel(ctx context.Context, input ModelInput) (Model, error)
	DeleteModel(ctx context.Context, id uuid.UUID) error
	ListModels(ctx context.Context, providerID *uuid.UUID, capability *Capability) ([]Model, error)
}

// ModelController serves /api/v1/models.
type ModelController struct {
	service      modelOperations
	maxBodyBytes int64
}

func NewModelController(service modelOperations, maxBodyBytes int64) *ModelController {
	return &ModelController{service: service, maxBodyBytes: maxBodyBytes}
}

// HandleListModels accepts optional providerId and capability query filters.
func (h *ModelController) HandleListModels(c *gin.Context) {
	providerID, err := optionalProviderID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	capability, err := optionalCapability(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	models, err := h.service.ListModels(c.Request.Context(), providerID, capability)
	httpx.Respond(c, http.StatusOK, newModelResponses(models), err)
}

func optionalProviderID(c *gin.Context) (*uuid.UUID, error) {
	raw := c.Query("providerId")
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errors.FieldError("providerId", errors.FieldCodeInvalid)
	}
	return &id, nil
}

func optionalCapability(c *gin.Context) (*Capability, error) {
	raw := c.Query("capability")
	if raw == "" {
		return nil, nil
	}
	capability := Capability(raw)
	if !IsValidCapability(capability) {
		return nil, errors.FieldError("capability", errors.FieldCodeInvalid)
	}
	return &capability, nil
}

func (h *ModelController) HandleCreateModel(c *gin.Context) {
	var body ModelRequest
	if err := httpx.DecodeJSON(c.Writer, c.Request, &body, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	model, err := h.service.CreateModel(c.Request.Context(), body.toInput())
	httpx.Respond(c, http.StatusCreated, newModelResponse(model), err)
}

func (h *ModelController) HandleDeleteModel(c *gin.Context) {
	id, err := pathUUID(c, "modelId")
	if err == nil {
		err = h.service.DeleteModel(c.Request.Context(), id)
	}
	httpx.Respond(c, http.StatusOK, DeleteResponse{Deleted: true}, err)
}
