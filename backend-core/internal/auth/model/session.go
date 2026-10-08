package model

import (
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
)

// Session is a stored login; Expired is computed by the database clock, not the app clock.
type Session struct {
	TenantID  string
	ID        uuid.UUID
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
	Expired   bool
}

// IssuedSession carries the raw tokens exactly once, at login.
type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

// HashSessionToken is the only form of the session token that is stored or looked up.
func HashSessionToken(sessionToken string) []byte {
	sum := sha256.Sum256([]byte(sessionToken))
	return sum[:]
}
