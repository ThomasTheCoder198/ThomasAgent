package outbound

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	schemeHTTPS    = "https"
	schemeHTTP     = "http"
	portHTTPS      = "443"
	portHTTP       = "80"
	maxPort        = 65535
	maxHostLength  = 253
	maxLabelLength = 63
)

func IsSupportedScheme(scheme string) bool { return scheme == schemeHTTPS || scheme == schemeHTTP }

type policy struct {
	privateDestinations map[string]bool
	blocked             []netip.Prefix
	ipv6Public          netip.Prefix
}

func newPolicy(allowlist []string) (*policy, error) {
	if err := ValidatePrivateAllowlist(allowlist); err != nil {
		return nil, err
	}
	p := &policy{privateDestinations: make(map[string]bool), ipv6Public: netip.MustParsePrefix("2000::/3")}
	for _, destination := range allowlist {
		p.privateDestinations[strings.ToLower(destination)] = true
	}
	// Global-unicast includes reserved/documentation space; exclude it explicitly.
	for _, prefix := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20",
	} {
		p.blocked = append(p.blocked, netip.MustParsePrefix(prefix))
	}
	return p, nil
}

func ValidatePrivateAllowlist(allowlist []string) error {
	for _, destination := range allowlist {
		host, port, err := net.SplitHostPort(destination)
		if err != nil || !validHost(host) || !validPort(port) || net.JoinHostPort(host, port) != destination {
			return errors.ErrValidationFailed.WithMessage("private allowlist requires exact hostname:port destinations")
		}
	}
	return nil
}

func validPort(port string) bool {
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= maxPort && strconv.Itoa(value) == port
}

func validHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Zone() == "" && !address.Is4In6()
	}
	if len(host) == 0 || len(host) > maxHostLength {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !validLabel(label) {
			return false
		}
	}
	return true
}

func validLabel(label string) bool {
	if len(label) == 0 || len(label) > maxLabelLength || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, character := range strings.ToLower(label) {
		if !validDNSCharacter(character) {
			return false
		}
	}
	return true
}

func validDNSCharacter(character rune) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-'
}

func (p *policy) validateURL(destination *url.URL) error {
	if !validURLAuthority(destination) {
		return errors.ErrRegistryProviderRejected.WithMessage("outbound URL is invalid")
	}
	if destination.Scheme != schemeHTTPS && destination.Scheme != schemeHTTP {
		return errors.ErrRegistryProviderRejected.WithMessage("outbound scheme is not permitted")
	}
	port := destination.Port()
	if port == "" {
		port = portHTTPS
		if destination.Scheme == schemeHTTP {
			port = portHTTP
		}
	}
	if !validPort(port) {
		return errors.ErrRegistryProviderRejected.WithMessage("outbound port is invalid")
	}
	if destination.Scheme == schemeHTTP && !p.privateDestinations[net.JoinHostPort(strings.ToLower(destination.Hostname()), port)] {
		return errors.ErrRegistryProviderRejected.WithMessage("outbound HTTP destination is not allowlisted")
	}
	return nil
}

func validURLAuthority(destination *url.URL) bool {
	return destination != nil && destination.User == nil && destination.Fragment == "" && destination.Opaque == "" && validHost(destination.Hostname())
}

func (p *policy) permitsAddress(address netip.Addr, privateAllowed bool) bool {
	if invalidAddress(address) {
		return false
	}
	if privateAllowed && (address.IsPrivate() || address.IsLoopback()) {
		return true
	}
	return p.permitsPublicAddress(address)
}

func invalidAddress(address netip.Addr) bool {
	return !address.IsValid() || address.Zone() != "" || address.Is4In6() || address.IsUnspecified() || address.IsMulticast()
}

func (p *policy) permitsPublicAddress(address netip.Addr) bool {
	if !address.IsGlobalUnicast() || (address.Is6() && !p.ipv6Public.Contains(address)) {
		return false
	}
	for _, prefix := range p.blocked {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
