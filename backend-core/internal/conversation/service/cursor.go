package service

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const cursorSeparator = "|"

// encodeCursor is opaque to clients: base64url("<updated_at RFC3339Nano>|<id>") (ruling A2-11).
func encodeCursor(updatedAt time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(updatedAt.UTC().Format(time.RFC3339Nano) + cursorSeparator + id.String()))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	invalid := errors.FieldError(model.FieldCursor, errors.FieldCodeInvalid)
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, invalid
	}
	at, idText, found := strings.Cut(string(raw), cursorSeparator)
	if !found {
		return time.Time{}, uuid.Nil, invalid
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, uuid.Nil, invalid
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return time.Time{}, uuid.Nil, invalid
	}
	return updatedAt, id, nil
}
