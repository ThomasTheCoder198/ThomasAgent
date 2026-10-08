package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type InternalModelController struct{ resolver service.Resolver }

func NewInternalModelController(resolver service.Resolver) *InternalModelController {
	return &InternalModelController{resolver: resolver}
}

func (h *InternalModelController) HandleResolve(c *gin.Context) {
	c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	out, err := h.resolver.Resolve(c.Request.Context(), model.Role(c.Query("role")))
	httpx.Respond(c, http.StatusOK, dto.FromResolvedModel(out), err)
}
