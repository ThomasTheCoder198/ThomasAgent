package model

import (
	"slices"
	"time"

	"github.com/google/uuid"

	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

// Limits match the CHECK constraints of the agents table (migration 20261017000001).
const (
	NameMaxRunes         = 80
	DescriptionMaxRunes  = 280
	InstructionsMaxRunes = 8000

	FieldName           = "name"
	FieldDescription    = "description"
	FieldInstructions   = "instructions"
	FieldKBID           = "kbId"
	FieldModelOverrides = "modelOverrides"
)

// OverridableRoles are the roles an Agent may pin to its own model (ruling A2-10).
var OverridableRoles = []llmmodel.Role{llmmodel.RoleChatDefault, llmmodel.RoleChatFast, llmmodel.RoleVision}

// Agent is one assistant of an org: instructions plus exactly one ragx Knowledge Base.
type Agent struct {
	ID             uuid.UUID
	Name           string
	Description    string
	Instructions   string
	KBID           uuid.UUID
	ModelOverrides map[llmmodel.Role]uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AgentInput struct {
	Name           string
	Description    string
	Instructions   string
	KBID           uuid.UUID
	ModelOverrides map[llmmodel.Role]uuid.UUID
}

// AgentPatch: a nil field is unchanged; in ModelOverrides a nil value removes that role's override.
type AgentPatch struct {
	Name           *string
	Description    *string
	Instructions   *string
	KBID           *uuid.UUID
	ModelOverrides map[llmmodel.Role]*uuid.UUID
}

func IsOverridable(role llmmodel.Role) bool { return slices.Contains(OverridableRoles, role) }
