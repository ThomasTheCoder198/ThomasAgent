package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
)

type CreateOrganizationRequest struct {
	Name            string `json:"name"`
	ExternalParsing *bool  `json:"externalParsing"`
}

func (r CreateOrganizationRequest) ToInput() model.OrganizationInput {
	return model.OrganizationInput(r)
}

type UpdateOrganizationRequest struct {
	Name            *string `json:"name"`
	ExternalParsing *bool   `json:"externalParsing"`
}

func (r UpdateOrganizationRequest) ToPatch() model.OrganizationPatch {
	return model.OrganizationPatch(r)
}

type OrganizationResponse struct {
	ID                     string           `json:"id"`
	Name                   string           `json:"name"`
	ExternalParsing        bool             `json:"externalParsing"`
	RagxStatus             model.RagxStatus `json:"ragxStatus"`
	RagxErrorCode          *string          `json:"ragxErrorCode"`
	SettingsSynced         bool             `json:"settingsSynced"`
	DefaultKnowledgeBaseID *uuid.UUID       `json:"defaultKnowledgeBaseId"`
	CreatedAt              time.Time        `json:"createdAt"`
	UpdatedAt              time.Time        `json:"updatedAt"`
}

// NewOrganizationResponse reports an empty error code as null: "" is the column's way of saying none.
func NewOrganizationResponse(o model.Organization) OrganizationResponse {
	var code *string
	if o.RagxErrorCode != "" {
		code = &o.RagxErrorCode
	}
	return OrganizationResponse{
		ID: o.ID, Name: o.Name, ExternalParsing: o.ExternalParsing, RagxStatus: o.RagxStatus, RagxErrorCode: code,
		SettingsSynced: o.SettingsSynced, DefaultKnowledgeBaseID: o.DefaultKnowledgeBaseID, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

// NewOrganizationResponses never returns nil: an empty list is [].
func NewOrganizationResponses(orgs []model.Organization) []OrganizationResponse {
	responses := make([]OrganizationResponse, 0, len(orgs))
	for _, org := range orgs {
		responses = append(responses, NewOrganizationResponse(org))
	}
	return responses
}
