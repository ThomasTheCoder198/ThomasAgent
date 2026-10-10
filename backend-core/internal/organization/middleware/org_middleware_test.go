package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type fakeLookup map[string]model.Organization

// GetOrganization behaves like the service: only the platform tenant sees orgs, and anything unknown is NOT_FOUND.
func (f fakeLookup) GetOrganization(ctx context.Context, id string) (model.Organization, error) {
	if scope, _ := tenant.ID(ctx); scope != tenant.PlatformID {
		return model.Organization{}, errors.ErrNotFound
	}
	org, ok := f[id]
	if !ok {
		return model.Organization{}, errors.ErrNotFound
	}
	return org, nil
}

func TestRequireOrg_RejectsPlatformUnknownMalformedAndUnreadyOrgs(t *testing.T) {
	ready, pending, failed := uuid.NewString(), uuid.NewString(), uuid.NewString()
	lookup := fakeLookup{
		ready:   {ID: ready, RagxStatus: model.RagxStatusReady},
		pending: {ID: pending, RagxStatus: model.RagxStatusPending},
		failed:  {ID: failed, RagxStatus: model.RagxStatusFailed},
	}
	var seenTenant string
	router := httpx.NewRouter(func(context.Context, *errors.AppError) {})
	platform := router.Group("", func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(tenant.WithID(ctx.Request.Context(), tenant.PlatformID))
	})
	platform.GET("/api/v1/orgs/:orgId/apps/ragx/*path", RequireOrg(lookup), func(ctx *gin.Context) {
		seenTenant, _ = tenant.ID(ctx.Request.Context())
		org, ok := OrganizationFromContext(ctx.Request.Context())
		require.True(t, ok)
		require.Equal(t, ready, org.ID)
		ctx.Status(http.StatusNoContent)
	})
	for id, want := range map[string]int{
		tenant.PlatformID: http.StatusNotFound,
		"not-a-uuid":      http.StatusNotFound,
		uuid.NewString():  http.StatusNotFound,
		pending:           http.StatusConflict,
		failed:            http.StatusConflict,
		ready:             http.StatusNoContent,
	} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+id+"/apps/ragx/knowledge-bases", nil))
		require.Equal(t, want, res.Code, id)
		if want == http.StatusConflict {
			require.Contains(t, res.Body.String(), `"ORG_NOT_PROVISIONED"`)
		}
	}
	require.Equal(t, ready, seenTenant, "RequireOrg is the only place a path org becomes the tenant")
}
