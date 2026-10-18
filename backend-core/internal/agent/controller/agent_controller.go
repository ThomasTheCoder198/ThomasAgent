package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// PathParamAgentID is the agent route parameter.
const PathParamAgentID = "agentId"

// AgentController serves /api/v1/orgs/{orgId}/agents; RequireOrg has already scoped the request to the org. A
// malformed agent id is AGENT_NOT_FOUND, like a missing one, so ids never leak through validation errors.
type AgentController struct {
	agentService service.AgentService
	maxBodyBytes int64
}

func NewAgentController(agentService service.AgentService, maxBodyBytes int64) *AgentController {
	return &AgentController{agentService: agentService, maxBodyBytes: maxBodyBytes}
}

func agentID(ctx *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(ctx.Param(PathParamAgentID))
	if err != nil {
		return uuid.Nil, errors.ErrAgentNotFound
	}
	return id, nil
}

func (c *AgentController) HandleListAgents(ctx *gin.Context) {
	agents, err := c.agentService.List(ctx.Request.Context())
	httpx.Respond(ctx, http.StatusOK, dto.NewAgentResponses(agents), err)
}

func (c *AgentController) HandleCreateAgent(ctx *gin.Context) {
	var request dto.CreateAgentRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	agent, err := c.agentService.Create(ctx.Request.Context(), request.ToInput())
	httpx.Respond(ctx, http.StatusCreated, dto.NewAgentResponse(agent), err)
}

func (c *AgentController) HandleGetAgent(ctx *gin.Context) {
	id, err := agentID(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	agent, err := c.agentService.Get(ctx.Request.Context(), id)
	httpx.Respond(ctx, http.StatusOK, dto.NewAgentResponse(agent), err)
}

func (c *AgentController) HandleUpdateAgent(ctx *gin.Context) {
	id, err := agentID(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	var request dto.UpdateAgentRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	agent, err := c.agentService.Update(ctx.Request.Context(), id, request.ToPatch())
	httpx.Respond(ctx, http.StatusOK, dto.NewAgentResponse(agent), err)
}

func (c *AgentController) HandleDeleteAgent(ctx *gin.Context) {
	id, err := agentID(ctx)
	if err == nil {
		err = c.agentService.Delete(ctx.Request.Context(), id)
	}
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
