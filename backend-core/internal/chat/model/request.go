package model

import (
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type Trigger string

const (
	TriggerSubmit     Trigger = "submit-message"
	TriggerRegenerate Trigger = "regenerate-message"
)

// UserMessage is the newest message the browser sent: its id, its parts as sent (stored verbatim) and their text.
type UserMessage struct {
	ID    string
	Text  string
	Parts []byte
}

// ChatRequest is what the controller hands the run service; tenant comes from the context, never from here.
type ChatRequest struct {
	ConversationID uuid.UUID
	AgentID        *uuid.UUID
	Trigger        Trigger
	MessageID      string
	Message        UserMessage
	ModelID        *uuid.UUID
	UserID         uuid.UUID
	Lang           errors.Lang
}
