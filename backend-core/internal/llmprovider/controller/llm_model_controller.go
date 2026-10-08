package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// LLMModelController serves /api/v1/models.
type LLMModelController struct {
	llmModelService service.LLMModelService
	maxBodyBytes    int64
}

func NewLLMModelController(llmModelService service.LLMModelService, maxBodyBytes int64) *LLMModelController {
	return &LLMModelController{llmModelService: llmModelService, maxBodyBytes: maxBodyBytes}
}

// HandleListModels accepts optional providerId and capability query filters.
func (c *LLMModelController) HandleListModels(ctx *gin.Context) {
	providerID, err := optionalProviderID(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	capability, err := optionalCapability(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	llmModels, err := c.llmModelService.ListModels(ctx.Request.Context(), providerID, capability)
	httpx.Respond(ctx, http.StatusOK, dto.NewLLMModelResponses(llmModels), err)
}

func (c *LLMModelController) HandleCreateModel(ctx *gin.Context) {
	var request dto.LLMModelRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	llmModel, err := c.llmModelService.CreateModel(ctx.Request.Context(), request.ToInput())
	httpx.Respond(ctx, http.StatusCreated, dto.NewLLMModelResponse(llmModel), err)
}

func (c *LLMModelController) HandleDeleteModel(ctx *gin.Context) {
	id, err := httpx.PathUUID(ctx, "modelId")
	if err == nil {
		err = c.llmModelService.DeleteModel(ctx.Request.Context(), id)
	}
	httpx.Respond(ctx, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}

func optionalProviderID(ctx *gin.Context) (*uuid.UUID, error) {
	raw := ctx.Query("providerId")
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errors.FieldError("providerId", errors.FieldCodeInvalid)
	}
	return &id, nil
}

func optionalCapability(ctx *gin.Context) (*model.Capability, error) {
	raw := ctx.Query("capability")
	if raw == "" {
		return nil, nil
	}
	capability := model.Capability(raw)
	if !model.IsValidCapability(capability) {
		return nil, errors.FieldError("capability", errors.FieldCodeInvalid)
	}
	return &capability, nil
}
