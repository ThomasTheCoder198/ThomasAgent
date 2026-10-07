package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func TestLimiterIdentity_UsesKeyedHMACAndNormalizesEmail(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	lim := authlimiter.NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, key)
	mac := hmac.New(sha256.New, key)
	_, err := mac.Write([]byte("thomas@example.com"))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), lim.Identity(" Thomas@Example.com "))
	other := authlimiter.NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("different-secret-key-at-least-32-chars"))
	require.NotEqual(t, lim.Identity(ownerEmail), other.Identity(ownerEmail))
	bare := sha256.Sum256([]byte(ownerEmail))
	require.NotEqual(t, hex.EncodeToString(bare[:]), lim.Identity(ownerEmail))
}
