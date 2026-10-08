package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// The browser holds two cookies: the HttpOnly session token and a readable CSRF token
// that the frontend echoes in CSRFHeader on unsafe methods.
const (
	SessionCookie = "thomas_session"
	CSRFCookie    = "thomas_csrf"
	CSRFHeader    = "X-CSRF-Token"
)

const sessionTokenBytes = 32

// Session is a stored login; Expired is computed by the database clock, not the app clock.
type Session struct {
	TenantID  string
	ID        uuid.UUID
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
	Expired   bool
}

// IssuedSession carries the raw tokens exactly once, at login; only the token hash is stored.
type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

func hashSessionToken(sessionToken string) []byte {
	sum := sha256.Sum256([]byte(sessionToken))
	return sum[:]
}

func newRandomToken() (string, error) {
	random := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
