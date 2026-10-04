package outbound

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const resolverNetwork = "ip"

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

func NewClient(timeout time.Duration, privateAllowlist []string) (*http.Client, error) {
	if timeout <= 0 {
		return nil, errors.ErrValidationFailed.WithMessage("outbound timeout must be positive")
	}
	p, err := newPolicy(privateAllowlist)
	if err != nil {
		return nil, err
	}
	lookup := &net.Resolver{}
	connection := &net.Dialer{Timeout: timeout}
	base := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return p.dialContext(ctx, network, address, lookup, connection)
		},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       timeout,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Timeout: timeout, Transport: &transport{policy: p, base: base}, CheckRedirect: rejectRedirect}, nil
}

func rejectRedirect(*http.Request, []*http.Request) error {
	return errors.ErrRegistryProviderRejected.WithMessage("outbound redirects are not permitted")
}

func (p *policy) dialContext(ctx context.Context, network, destination string, lookup resolver, connection dialer) (net.Conn, error) {
	host, port, err := net.SplitHostPort(destination)
	if err != nil || !validHost(host) || !validPort(port) {
		return nil, errors.ErrRegistryProviderRejected
	}
	addresses, err := resolveAddresses(ctx, host, lookup)
	if err != nil {
		return nil, safeNetworkError(ctx, err)
	}
	privateAllowed := p.privateDestinations[net.JoinHostPort(strings.ToLower(host), port)]
	if len(addresses) == 0 {
		return nil, errors.ErrProviderUnavailable
	}
	for _, address := range addresses {
		if !p.permitsAddress(address, privateAllowed) {
			return nil, errors.ErrRegistryProviderRejected.WithMessage("outbound address is not permitted")
		}
	}
	// Dial a checked IP rather than the hostname so DNS cannot change between validation and connection.
	var lastDialError error
	for _, address := range addresses {
		conn, err := connection.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return conn, nil
		}
		lastDialError = err
	}
	return nil, safeNetworkError(ctx, lastDialError)
}

func resolveAddresses(ctx context.Context, host string, lookup resolver) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	return lookup.LookupNetIP(ctx, resolverNetwork, host)
}

func safeNetworkError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errors.ErrProviderUnavailable.WithCause(ctx.Err())
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return errors.ErrDependencyTimeout
	}
	return errors.ErrProviderUnavailable
}

type transport struct {
	policy *policy
	base   *http.Transport
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.policy.validateURL(req.URL); err != nil {
		return nil, err
	}
	res, err := t.base.RoundTrip(req)
	if err != nil {
		var appErr *errors.AppError
		if stderrors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, safeNetworkError(req.Context(), err)
	}
	res.Body = safeBody{ReadCloser: res.Body}
	return res, nil
}

func (t *transport) CloseIdleConnections() { t.base.CloseIdleConnections() }

type safeBody struct{ io.ReadCloser }

func (b safeBody) Read(buffer []byte) (int, error) {
	count, err := b.ReadCloser.Read(buffer)
	if err != nil && !stderrors.Is(err, io.EOF) {
		return count, errors.ErrProviderUnavailable
	}
	return count, err
}

func (b safeBody) Close() error {
	if err := b.ReadCloser.Close(); err != nil {
		return errors.ErrProviderUnavailable
	}
	return nil
}
