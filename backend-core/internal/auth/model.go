package auth

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
}

type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}
