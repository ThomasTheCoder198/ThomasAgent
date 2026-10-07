package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const ActionLoginFailed = "auth.login.failed"

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type User struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
}

type Session struct {
	TenantID  string
	ID        uuid.UUID
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
	Expired   bool
}

type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}
