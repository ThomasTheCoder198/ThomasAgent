package auth

import (
	"strings"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
}

// NormalizeEmail makes lookups and limiter identities ignore case and surrounding spaces.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
