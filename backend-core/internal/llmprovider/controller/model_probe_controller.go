package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const pathModelID = "modelId"

// ModelProbeController serves POST /api/v1/models/{modelId}/test.
type ModelProbeController struct {
	probes service.ModelProbeService
}

func NewModelProbeController(probes service.ModelProbeService) *ModelProbeController {
	return &ModelProbeController{probes: probes}
}

func (c *ModelProbeController) HandleTestModel(ctx *gin.Context) {
	modelID, err := httpx.PathUUID(ctx, pathModelID)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	result, err := c.probes.Probe(ctx.Request.Context(), modelID)
	httpx.Respond(ctx, http.StatusOK, dto.ModelProbeResponse{OK: true, LatencyMS: result.LatencyMS}, err)
}
