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

// RoleController serves /api/v1/model-roles.
type RoleController struct {
	roleService  service.RoleService
	maxBodyBytes int64
}

func NewRoleController(roleService service.RoleService, maxBodyBytes int64) *RoleController {
	return &RoleController{roleService: roleService, maxBodyBytes: maxBodyBytes}
}

func (c *RoleController) HandleListRoles(ctx *gin.Context) {
	assignments, err := c.roleService.ListRoles(ctx.Request.Context())
	httpx.Respond(ctx, http.StatusOK, dto.NewRoleAssignmentResponses(assignments), err)
}

func (c *RoleController) HandleAssignRole(ctx *gin.Context) {
	var request dto.AssignRoleRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	role := model.Role(ctx.Param("role"))
	if request.ModelID == uuid.Nil {
		httpx.Fail(ctx, errors.FieldError("modelId", errors.FieldCodeRequired))
		return
	}
	err := c.roleService.AssignRole(ctx.Request.Context(), role, request.ModelID)
	httpx.Respond(ctx, http.StatusOK, dto.RoleAssignmentResponse{Role: role, ModelID: request.ModelID}, err)
}
