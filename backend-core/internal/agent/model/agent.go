package model

import (
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
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
	KBName         string
	KBMissing      bool
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

// NormalizeInput trims the name and checks the stored limits without echoing values.
func NormalizeInput(in *AgentInput) error {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return errors.FieldError(FieldName, errors.FieldCodeRequired)
	case utf8.RuneCountInString(in.Name) > NameMaxRunes:
		return errors.FieldError(FieldName, errors.FieldCodeInvalid)
	case utf8.RuneCountInString(in.Description) > DescriptionMaxRunes:
		return errors.FieldError(FieldDescription, errors.FieldCodeInvalid)
	case utf8.RuneCountInString(in.Instructions) > InstructionsMaxRunes:
		return errors.FieldError(FieldInstructions, errors.FieldCodeInvalid)
	case in.KBID == uuid.Nil:
		return errors.FieldError(FieldKBID, errors.FieldCodeRequired)
	}
	return nil
}

// Apply returns a patch as a complete input; nil override values remove their roles.
func (a Agent) Apply(patch AgentPatch) AgentInput {
	in := AgentInput{Name: a.Name, Description: a.Description, Instructions: a.Instructions, KBID: a.KBID,
		ModelOverrides: maps.Clone(a.ModelOverrides)}
	if in.ModelOverrides == nil {
		in.ModelOverrides = map[llmmodel.Role]uuid.UUID{}
	}
	if patch.Name != nil {
		in.Name = *patch.Name
	}
	if patch.Description != nil {
		in.Description = *patch.Description
	}
	if patch.Instructions != nil {
		in.Instructions = *patch.Instructions
	}
	if patch.KBID != nil {
		in.KBID = *patch.KBID
	}
	for role, id := range patch.ModelOverrides {
		if id == nil {
			delete(in.ModelOverrides, role)
		} else {
			in.ModelOverrides[role] = *id
		}
	}
	return in
}
