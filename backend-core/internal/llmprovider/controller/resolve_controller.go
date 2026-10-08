package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// ResolveController serves the internal GET /internal/models/resolve?role=... route.
type ResolveController struct {
	resolveService service.ResolveService
}

func NewResolveController(resolveService service.ResolveService) *ResolveController {
	return &ResolveController{resolveService: resolveService}
}

func (c *ResolveController) HandleResolve(ctx *gin.Context) {
	ctx.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	resolved, err := c.resolveService.Resolve(ctx.Request.Context(), model.Role(ctx.Query("role")))
	httpx.Respond(ctx, http.StatusOK, dto.NewResolvedModelResponse(resolved), err)
}
