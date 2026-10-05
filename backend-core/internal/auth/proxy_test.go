package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func TestLoginProxyTrust_SelectsRightmostUntrustedHop(t *testing.T) {
	h := NewHandlerWithProxyHeader(nil, nil, false, "X-Forwarded-For", nil)
	h.trustedProxyCIDRs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:1::/48")}
	for _, test := range []struct{ peer, header, want string }{
		{"192.0.2.10:1234", "198.51.100.1", "192.0.2.10"},
		{"10.0.0.1:1234", "198.51.100.1, 192.0.2.8, 10.0.0.2", "192.0.2.8"},
		{"10.0.0.1:1234", "198.51.100.1, attacker-controlled", "10.0.0.1"},
		{"10.0.0.1:1234", "10.0.0.2", "10.0.0.1"},
		{"[2001:db8:1::1]:1234", "2001:db8:2::2, 2001:db8:1::2", "2001:db8:2::2"},
	} {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("X-Forwarded-For", test.header)
		require.Equal(t, test.want, h.clientIP(r))
	}
}

func TestLimiterIdentity_UsesKeyedHMACAndNormalizesEmail(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	lim := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, key)
	mac := hmac.New(sha256.New, key)
	_, err := mac.Write([]byte("thomas@example.com"))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), lim.identity(" Thomas@Example.com "))
	other := NewLoginLimiter(nil, config.AuthConfig{LoginMaxAttempts: 1, LoginEmailMaxAttempts: 2, LoginIPMaxAttempts: 100, LoginWindow: time.Minute}, []byte("different-secret-key-at-least-32-chars"))
	require.NotEqual(t, lim.identity(ownerEmail), other.identity(ownerEmail))
	bare := sha256.Sum256([]byte(ownerEmail))
	require.NotEqual(t, hex.EncodeToString(bare[:]), lim.identity(ownerEmail))
}
