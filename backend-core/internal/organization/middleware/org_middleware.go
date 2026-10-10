package middleware

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const PathParamOrgID = "orgId"

// OrganizationLookup is satisfied by service.OrganizationService.
type OrganizationLookup interface {
	GetOrganization(ctx context.Context, id string) (model.Organization, error)
}

type organizationKey struct{}

func OrganizationFromContext(ctx context.Context) (model.Organization, bool) {
	org, ok := ctx.Value(organizationKey{}).(model.Organization)
	return org, ok
}

// RequireOrg is the only place a path org becomes tenant context (rule 11). It runs after RequireSession, looks the
// org up under the session's platform tenant, refuses an org ragx has not provisioned (no ragx call), then scopes
// the request to the org: the App token's oid comes from this tenant.
func RequireOrg(lookup OrganizationLookup) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		org, err := lookup.GetOrganization(ctx.Request.Context(), ctx.Param(PathParamOrgID))
		if err != nil {
			httpx.Fail(ctx, err)
			return
		}
		if org.RagxStatus != model.RagxStatusReady {
			httpx.Fail(ctx, errors.ErrOrgNotProvisioned)
			return
		}
		scoped := context.WithValue(tenant.WithID(ctx.Request.Context(), org.ID), organizationKey{}, org)
		ctx.Request = ctx.Request.WithContext(scoped)
		ctx.Next()
	}
}
