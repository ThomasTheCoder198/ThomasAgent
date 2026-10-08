package controller

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const invalidPeerIdentity = "invalid-peer"

// clientIP chooses the address the login limiter counts. A forwarding header is honored only
// when the direct peer is a trusted proxy; the rightmost untrusted hop is the real client.
func (c *AuthController) clientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return invalidPeerIdentity
	}
	peer = peer.Unmap()
	if c.trustedProxyHeader == "" || !c.isTrustedProxy(peer) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(req.Header.Values(c.trustedProxyHeader), ","), ",")
	for index := len(hops) - 1; index >= 0; index-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if err != nil {
			return peer.String()
		}
		hop = hop.Unmap()
		if !c.isTrustedProxy(hop) {
			return hop.String()
		}
	}
	return peer.String()
}

func (c *AuthController) isTrustedProxy(ip netip.Addr) bool {
	for _, network := range c.trustedProxyCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
