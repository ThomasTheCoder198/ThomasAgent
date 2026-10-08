package dto

import (
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

type AssignRoleRequest struct {
	ModelID uuid.UUID `json:"modelId"`
}

type RoleAssignmentResponse struct {
	Role    model.Role `json:"role"`
	ModelID uuid.UUID  `json:"modelId"`
}

func NewRoleAssignmentResponses(assignments []model.RoleAssignment) []RoleAssignmentResponse {
	responses := make([]RoleAssignmentResponse, 0, len(assignments))
	for _, assignment := range assignments {
		responses = append(responses, RoleAssignmentResponse(assignment))
	}
	return responses
}
