package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

type CreateAgentRequest struct {
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Instructions   string               `json:"instructions"`
	KBID           uuid.UUID            `json:"kbId"`
	ModelOverrides map[string]uuid.UUID `json:"modelOverrides"`
}

func (r CreateAgentRequest) ToInput() model.AgentInput {
	overrides := make(map[llmmodel.Role]uuid.UUID, len(r.ModelOverrides))
	for role, id := range r.ModelOverrides {
		overrides[llmmodel.Role(role)] = id
	}
	return model.AgentInput{Name: r.Name, Description: r.Description, Instructions: r.Instructions, KBID: r.KBID, ModelOverrides: overrides}
}

// UpdateAgentRequest: absent fields are unchanged; a null modelOverrides value removes that role's override.
type UpdateAgentRequest struct {
	Name           *string               `json:"name"`
	Description    *string               `json:"description"`
	Instructions   *string               `json:"instructions"`
	KBID           *uuid.UUID            `json:"kbId"`
	ModelOverrides map[string]*uuid.UUID `json:"modelOverrides"`
}

func (r UpdateAgentRequest) ToPatch() model.AgentPatch {
	patch := model.AgentPatch{Name: r.Name, Description: r.Description, Instructions: r.Instructions, KBID: r.KBID}
	if r.ModelOverrides != nil {
		patch.ModelOverrides = make(map[llmmodel.Role]*uuid.UUID, len(r.ModelOverrides))
		for role, id := range r.ModelOverrides {
			patch.ModelOverrides[llmmodel.Role(role)] = id
		}
	}
	return patch
}

type AgentResponse struct {
	ID             uuid.UUID            `json:"id"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Instructions   string               `json:"instructions"`
	KBID           uuid.UUID            `json:"kbId"`
	KBName         string               `json:"kbName"`
	KBMissing      bool                 `json:"kbMissing"`
	ModelOverrides map[string]uuid.UUID `json:"modelOverrides"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
}

func NewAgentResponse(a model.Agent) AgentResponse {
	overrides := make(map[string]uuid.UUID, len(a.ModelOverrides))
	for role, id := range a.ModelOverrides {
		overrides[string(role)] = id
	}
	return AgentResponse{ID: a.ID, Name: a.Name, Description: a.Description, Instructions: a.Instructions, KBID: a.KBID,
		KBName: a.KBName, KBMissing: a.KBMissing, ModelOverrides: overrides, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

// NewAgentResponses never returns nil: an empty list is [].
func NewAgentResponses(agents []model.Agent) []AgentResponse {
	out := make([]AgentResponse, 0, len(agents))
	for _, a := range agents {
		out = append(out, NewAgentResponse(a))
	}
	return out
}
