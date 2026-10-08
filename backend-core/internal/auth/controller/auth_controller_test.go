package controller

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginProxyTrustSelectsRightmostUntrustedHop(t *testing.T) {
	h := NewAuthController(AuthControllerParams{TrustedProxyHeader: "X-Forwarded-For", TrustedProxyCIDRs: []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:1::/48"),
	}})
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

func TestClientIPJoinsForwardingHeaderLines(t *testing.T) {
	h := NewAuthController(AuthControllerParams{TrustedProxyHeader: "X-Forwarded-For", TrustedProxyCIDRs: []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	}})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "10.0.0.1:9000"
	req.Header.Add("X-Forwarded-For", "192.0.2.99")
	req.Header.Add("X-Forwarded-For", "198.51.100.7, 10.0.0.2")
	require.Equal(t, "198.51.100.7", h.clientIP(req))
}
