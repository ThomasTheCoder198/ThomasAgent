package model

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type RagxStatus string

const (
	RagxStatusPending RagxStatus = "pending"
	RagxStatusReady   RagxStatus = "ready"
	RagxStatusFailed  RagxStatus = "failed"

	// NameMaxLength is ragx's OrgProvision.name maxLength (contracts/openapi/ragx.v1.yaml).
	NameMaxLength = 200
	FieldName     = "name"
)

// Settings are what core sends to ragx; a sync result applies only while the row still holds them (ruling P3).
type Settings struct {
	Name            string
	ExternalParsing bool
}

type Organization struct {
	ID string
	Settings
	RagxStatus             RagxStatus
	RagxErrorCode          string
	DefaultKnowledgeBaseID *uuid.UUID
	SettingsSynced         bool
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type OrganizationInput struct {
	Name            string
	ExternalParsing *bool
}

type OrganizationPatch struct {
	Name            *string
	ExternalParsing *bool
}

// NormalizeName trims and checks a display name; the limit counts characters, as ragx's does.
func NormalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", errors.FieldError(FieldName, errors.FieldCodeRequired)
	case utf8.RuneCountInString(name) > NameMaxLength || strings.ContainsFunc(name, unicode.IsControl):
		return "", errors.FieldError(FieldName, errors.FieldCodeInvalid)
	}
	return name, nil
}
