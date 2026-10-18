package dto

import (
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

type OrgRoleResponse struct {
	Role             model.Role `json:"role"`
	ModelID          *uuid.UUID `json:"modelId"`
	EffectiveModelID *uuid.UUID `json:"effectiveModelId"`
	Source           string     `json:"source"`
}

func NewOrgRoleResponses(views []model.OrgRoleView) []OrgRoleResponse {
	responses := make([]OrgRoleResponse, 0, len(views))
	for _, view := range views {
		responses = append(responses, OrgRoleResponse(view))
	}
	return responses
}
