package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// OrganizationController serves /api/v1/orgs. The path id goes to the service as a string: a malformed id must be
// NOT_FOUND, not VALIDATION_FAILED, so httpx.PathUUID is not used.
type OrganizationController struct {
	organizationService service.OrganizationService
	maxBodyBytes        int64
}

func NewOrganizationController(organizationService service.OrganizationService, maxBodyBytes int64) *OrganizationController {
	return &OrganizationController{organizationService: organizationService, maxBodyBytes: maxBodyBytes}
}

func (c *OrganizationController) HandleListOrganizations(ctx *gin.Context) {
	orgs, err := c.organizationService.ListOrganizations(ctx.Request.Context())
	httpx.Respond(ctx, http.StatusOK, dto.NewOrganizationResponses(orgs), err)
}

func (c *OrganizationController) HandleCreateOrganization(ctx *gin.Context) {
	var request dto.CreateOrganizationRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	org, err := c.organizationService.CreateOrganization(ctx.Request.Context(), request.ToInput())
	httpx.Respond(ctx, http.StatusCreated, dto.NewOrganizationResponse(org), err)
}

func (c *OrganizationController) HandleGetOrganization(ctx *gin.Context) {
	org, err := c.organizationService.GetOrganization(ctx.Request.Context(), ctx.Param(middleware.PathParamOrgID))
	httpx.Respond(ctx, http.StatusOK, dto.NewOrganizationResponse(org), err)
}

func (c *OrganizationController) HandleUpdateOrganization(ctx *gin.Context) {
	var request dto.UpdateOrganizationRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	org, err := c.organizationService.UpdateOrganization(ctx.Request.Context(), ctx.Param(middleware.PathParamOrgID), request.ToPatch())
	httpx.Respond(ctx, http.StatusOK, dto.NewOrganizationResponse(org), err)
}

// HandleProvisionOrganization answers 200 with the org's real state, even when ragx failed (ruling P1).
func (c *OrganizationController) HandleProvisionOrganization(ctx *gin.Context) {
	org, err := c.organizationService.ProvisionOrganization(ctx.Request.Context(), ctx.Param(middleware.PathParamOrgID))
	httpx.Respond(ctx, http.StatusOK, dto.NewOrganizationResponse(org), err)
}
