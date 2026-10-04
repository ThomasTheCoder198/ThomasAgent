package outbound

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type fakeResolver struct {
	addresses []netip.Addr
	err       error
	calls     int
}

func (r *fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.calls++
	return r.addresses, r.err
}

type fakeDialer struct {
	address string
	calls   int
	err     error
}

func (d *fakeDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.address = address
	d.calls++
	return nil, d.err
}

func TestPolicy_RejectsUnsafeURLsBeforeNetwork(t *testing.T) {
	p, err := newPolicy(nil)
	require.NoError(t, err)
	for _, raw := range []string{"http://api.example/models", "https://user:secret@api.example/models", "https://api.example/models#secret", "ftp://api.example/models", "https://api.example:0/models", "https://[fe80::1%25eth0]/models"} {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			require.NoError(t, err)
			require.Error(t, p.validateURL(u))
		})
	}
	u, err := url.Parse("https://api.example/models")
	require.NoError(t, err)
	require.NoError(t, p.validateURL(u))
}

func TestPolicy_PrivateAllowlistMatchesExactDestination(t *testing.T) {
	p, err := newPolicy([]string{"localhost:11434"})
	require.NoError(t, err)
	for raw, allowed := range map[string]bool{"http://localhost:11434/models": true, "http://localhost:80/models": false, "http://sub.localhost:11434/models": false, "http://127.0.0.1:11434/models": false} {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		if allowed {
			require.NoError(t, p.validateURL(u))
		} else {
			require.Error(t, p.validateURL(u))
		}
	}
}

func TestValidatePrivateAllowlist_RejectsMalformedEntries(t *testing.T) {
	for _, entry := range []string{"*", "*.example:443", "https://localhost:11434", "localhost", "localhost:0", "localhost:65536", "localhost:abc", "localhost:080", "host/name:80", "host@name:80", "[fe80::1%eth0]:80", "host..name:80", "-host:80", "host.:80", ""} {
		require.Error(t, ValidatePrivateAllowlist([]string{entry}), entry)
	}
	require.NoError(t, ValidatePrivateAllowlist([]string{"localhost:11434", "[::1]:8080", "192.168.1.2:8000", "API.EXAMPLE:443"}))
}

func TestDial_RejectsAllNonpublicAddressesAndMixedDNS(t *testing.T) {
	unsafe := []string{"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.0.1", "192.0.0.1", "192.0.2.1", "192.88.99.1", "192.168.0.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "::", "::1", "::ffff:8.8.8.8", "fc00::1", "fe80::1", "ff02::1", "64:ff9b::a00:1", "2001:db8::1", "2001::1", "2002:0808:0808::1", "3fff::1"}
	for _, address := range unsafe {
		t.Run(address, func(t *testing.T) {
			p, err := newPolicy(nil)
			require.NoError(t, err)
			r := &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(address)}}
			d := &fakeDialer{}
			_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
			require.Error(t, err)
			require.Zero(t, d.calls)
		})
	}
}

func TestDial_PinsCheckedIPWithoutSecondDNSLookup(t *testing.T) {
	p, err := newPolicy(nil)
	require.NoError(t, err)
	r := &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	d := &fakeDialer{}
	_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:443", d.address)
	require.Equal(t, 1, r.calls)
	r.addresses = []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111")}
	_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
	require.NoError(t, err)
	require.Equal(t, "[2606:4700:4700::1111]:443", d.address)
}

func TestDial_ExplicitPrivatePermissionStillRejectsMetadataAndMappedIP(t *testing.T) {
	p, err := newPolicy([]string{"localhost:11434"})
	require.NoError(t, err)
	for address, allowed := range map[string]bool{"127.0.0.1": true, "10.0.0.1": true, "::1": true, "fc00::1": true, "169.254.169.254": false, "::ffff:127.0.0.1": false, "0.0.0.0": false} {
		r := &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr(address)}}
		d := &fakeDialer{}
		_, err := p.dialContext(t.Context(), "tcp", "localhost:11434", r, d)
		if allowed {
			require.NoError(t, err)
			require.Equal(t, 1, d.calls)
		} else {
			require.Error(t, err)
			require.Zero(t, d.calls)
		}
	}
}

func TestDial_FailuresNeverIncludeResolverOrDialSecrets(t *testing.T) {
	p, err := newPolicy(nil)
	require.NoError(t, err)
	r := &fakeResolver{err: stderrors.New("password=secret-key")}
	d := &fakeDialer{}
	_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-key")
	r.err = nil
	r.addresses = []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	d.err = stderrors.New("secret-key")
	_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-key")
}

func TestClient_RejectsRedirectsAndIgnoresAmbientProxies(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	client, err := NewClient(time.Second, nil)
	require.NoError(t, err)
	guard := client.Transport.(*transport)
	require.Nil(t, guard.base.Proxy)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.example/models", nil)
	require.NoError(t, err)
	require.Error(t, client.CheckRedirect(req, nil))
	require.Equal(t, time.Second, client.Timeout)
	require.Error(t, func() error { _, err := NewClient(0, nil); return err }())
}

type secretBody struct{}

func (secretBody) Read([]byte) (int, error) { return 0, stderrors.New("raw-secret") }
func (secretBody) Close() error             { return stderrors.New("raw-secret") }
func TestSafeBody_DiscardsRawReadAndCloseErrors(t *testing.T) {
	body := safeBody{ReadCloser: secretBody{}}
	_, err := io.ReadAll(body)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "raw-secret")
	err = body.Close()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "raw-secret")
	body = safeBody{ReadCloser: io.NopCloser(strings.NewReader("ok"))}
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
}

func TestClient_RejectsRedirectWithoutForwardingAuthorization(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { targetCalls++; w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	u, err := url.Parse(source.URL)
	require.NoError(t, err)
	client, err := NewClient(time.Second, []string{u.Host})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, source.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-secret")
	res, err := client.Do(req)
	if res != nil {
		require.NoError(t, res.Body.Close())
	}
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrRegistryProviderRejected)
	require.Zero(t, targetCalls)
}

func TestTransport_RejectsURLBeforeDialAndSanitizesTLSFailure(t *testing.T) {
	client, err := NewClient(time.Second, nil)
	require.NoError(t, err)
	guard := client.Transport.(*transport)
	dials := 0
	guard.base.DialContext = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return nil, stderrors.New("tls-password=raw-secret")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://user:raw-secret@api.example", nil)
	require.NoError(t, err)
	res, err := guard.RoundTrip(req)
	if res != nil {
		require.NoError(t, res.Body.Close())
	}
	require.ErrorIs(t, err, errors.ErrRegistryProviderRejected)
	require.Zero(t, dials)
	require.NotContains(t, err.Error(), "raw-secret")
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.example", nil)
	require.NoError(t, err)
	res, err = guard.RoundTrip(req)
	if res != nil {
		require.NoError(t, res.Body.Close())
	}
	require.ErrorIs(t, err, errors.ErrProviderUnavailable)
	require.Equal(t, 1, dials)
	require.NotContains(t, err.Error(), "raw-secret")
}

func TestDial_IPLiteralsAvoidDNSAndEmptyDNSFailsClosed(t *testing.T) {
	p, err := newPolicy(nil)
	require.NoError(t, err)
	r := &fakeResolver{}
	d := &fakeDialer{}
	_, err = p.dialContext(t.Context(), "tcp", "8.8.8.8:443", r, d)
	require.NoError(t, err)
	require.Zero(t, r.calls)
	_, err = p.dialContext(t.Context(), "tcp", "127.0.0.1:443", r, d)
	require.ErrorIs(t, err, errors.ErrRegistryProviderRejected)
	require.Equal(t, 1, d.calls)
	_, err = p.dialContext(t.Context(), "tcp", "api.example:443", r, d)
	require.ErrorIs(t, err, errors.ErrProviderUnavailable)
	require.Equal(t, 1, d.calls)
}
