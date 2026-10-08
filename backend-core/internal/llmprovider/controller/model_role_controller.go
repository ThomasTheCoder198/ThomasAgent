package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type modelRoleOperations interface {
	AssignRole(context.Context, model.Role, uuid.UUID) error
	ListRoles(context.Context) ([]model.RoleAssignment, error)
}

type ModelRoleController struct {
	svc          modelRoleOperations
	maxBodyBytes int64
}

func NewModelRoleController(svc modelRoleOperations, maxBodyBytes int64) *ModelRoleController {
	return &ModelRoleController{
		svc:          svc,
		maxBodyBytes: maxBodyBytes,
	}
}

func (h *ModelRoleController) HandleListRoles(c *gin.Context) {
	r := c.Request
	out, err := h.svc.ListRoles(r.Context())
	httpx.Respond(c, http.StatusOK, dto.FromRoles(out), err)
}

func (h *ModelRoleController) HandleAssignRole(c *gin.Context) {
	w, r := c.Writer, c.Request
	var body dto.AssignRoleRequest
	if err := httpx.DecodeJSON(w, r, &body, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	role := model.Role(c.Param("role"))
	if body.ModelID == uuid.Nil {
		httpx.Fail(c, errors.FieldError("modelId", errors.FieldCodeRequired))
		return
	}
	err := h.svc.AssignRole(r.Context(), role, body.ModelID)
	httpx.Respond(c, http.StatusOK, dto.RoleAssignmentResponse{Role: role, ModelID: body.ModelID}, err)
}
