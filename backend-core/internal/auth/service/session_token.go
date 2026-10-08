package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const sessionTokenBytes = 32

// newRandomToken makes session and CSRF tokens: 32 random bytes, URL-safe base64.
func newRandomToken() (string, error) {
	random := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
