package llm

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type AssignRoleRequest struct {
	ModelID uuid.UUID `json:"modelId"`
}

type RoleAssignmentResponse struct {
	Role    Role      `json:"role"`
	ModelID uuid.UUID `json:"modelId"`
}

func newRoleAssignmentResponses(assignments []RoleAssignment) []RoleAssignmentResponse {
	responses := make([]RoleAssignmentResponse, 0, len(assignments))
	for _, assignment := range assignments {
		responses = append(responses, RoleAssignmentResponse(assignment))
	}
	return responses
}

type roleOperations interface {
	AssignRole(ctx context.Context, role Role, modelID uuid.UUID) error
	ListRoles(ctx context.Context) ([]RoleAssignment, error)
}

// RoleController serves /api/v1/model-roles.
type RoleController struct {
	service      roleOperations
	maxBodyBytes int64
}

func NewRoleController(service roleOperations, maxBodyBytes int64) *RoleController {
	return &RoleController{service: service, maxBodyBytes: maxBodyBytes}
}

func (h *RoleController) HandleListRoles(c *gin.Context) {
	assignments, err := h.service.ListRoles(c.Request.Context())
	httpx.Respond(c, http.StatusOK, newRoleAssignmentResponses(assignments), err)
}

func (h *RoleController) HandleAssignRole(c *gin.Context) {
	var body AssignRoleRequest
	if err := httpx.DecodeJSON(c.Writer, c.Request, &body, h.maxBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	role := Role(c.Param("role"))
	if body.ModelID == uuid.Nil {
		httpx.Fail(c, errors.FieldError("modelId", errors.FieldCodeRequired))
		return
	}
	err := h.service.AssignRole(c.Request.Context(), role, body.ModelID)
	httpx.Respond(c, http.StatusOK, RoleAssignmentResponse{Role: role, ModelID: body.ModelID}, err)
}
