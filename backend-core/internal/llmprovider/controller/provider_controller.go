package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// ProviderController serves /api/v1/providers.
type ProviderController struct {
	providerService  service.ProviderService
	modelSyncService service.ModelSyncService
	maxBodyBytes     int64
}

func NewProviderController(providerService service.ProviderService, modelSyncService service.ModelSyncService, maxBodyBytes int64) *ProviderController {
	return &ProviderController{providerService: providerService, modelSyncService: modelSyncService, maxBodyBytes: maxBodyBytes}
}

func (c *ProviderController) HandleListProviders(ctx *gin.Context) {
	providers, err := c.providerService.ListProviders(ctx.Request.Context())
	httpx.Respond(ctx, http.StatusOK, dto.NewProviderResponses(providers), err)
}

func (c *ProviderController) HandleCreateProvider(ctx *gin.Context) {
	var request dto.ProviderRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	provider, err := c.providerService.CreateProvider(ctx.Request.Context(), request.ToInput())
	httpx.Respond(ctx, http.StatusCreated, dto.NewProviderResponse(provider), err)
}

func (c *ProviderController) HandleUpdateProvider(ctx *gin.Context) {
	id, err := httpx.PathUUID(ctx, "providerId")
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	var request dto.ProviderRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	provider, err := c.providerService.UpdateProvider(ctx.Request.Context(), id, request.ToInput())
	httpx.Respond(ctx, http.StatusOK, dto.NewProviderResponse(provider), err)
}

func (c *ProviderController) HandleDeleteProvider(ctx *gin.Context) {
	id, err := httpx.PathUUID(ctx, "providerId")
	if err == nil {
		err = c.providerService.DeleteProvider(ctx.Request.Context(), id)
	}
	httpx.Respond(ctx, http.StatusOK, dto.DeleteResponse{Deleted: true}, err)
}

func (c *ProviderController) HandleTestProvider(ctx *gin.Context) {
	id, err := httpx.PathUUID(ctx, "providerId")
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	modelCount, err := c.providerService.TestProvider(ctx.Request.Context(), id)
	httpx.Respond(ctx, http.StatusOK, dto.TestProviderResponse{OK: true, ModelCount: modelCount}, err)
}

func (c *ProviderController) HandleSyncProvider(ctx *gin.Context) {
	id, err := httpx.PathUUID(ctx, "providerId")
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	result, err := c.modelSyncService.SyncModels(ctx.Request.Context(), id)
	httpx.Respond(ctx, http.StatusOK, dto.NewSyncResultResponse(result), err)
}
