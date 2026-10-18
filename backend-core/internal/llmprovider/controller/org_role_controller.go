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

// OrgRoleController serves /api/v1/orgs/{orgId}/model-roles; RequireOrg has already scoped the request to the org.
type OrgRoleController struct {
	orgRoleService service.OrgRoleService
	maxBodyBytes   int64
}

func NewOrgRoleController(orgRoleService service.OrgRoleService, maxBodyBytes int64) *OrgRoleController {
	return &OrgRoleController{orgRoleService: orgRoleService, maxBodyBytes: maxBodyBytes}
}

func (c *OrgRoleController) HandleListOrgRoles(ctx *gin.Context) {
	views, err := c.orgRoleService.ListOrgRoles(ctx.Request.Context())
	httpx.Respond(ctx, http.StatusOK, dto.NewOrgRoleResponses(views), err)
}

func (c *OrgRoleController) HandleAssignOrgRole(ctx *gin.Context) {
	var request dto.AssignRoleRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	if request.ModelID == uuid.Nil {
		httpx.Fail(ctx, errors.FieldError("modelId", errors.FieldCodeRequired))
		return
	}
	role := model.Role(ctx.Param("role"))
	err := c.orgRoleService.AssignOrgRole(ctx.Request.Context(), role, request.ModelID)
	httpx.Respond(ctx, http.StatusOK, dto.RoleAssignmentResponse{Role: role, ModelID: request.ModelID}, err)
}

func (c *OrgRoleController) HandleClearOrgRole(ctx *gin.Context) {
	if err := c.orgRoleService.ClearOrgRole(ctx.Request.Context(), model.Role(ctx.Param("role"))); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
