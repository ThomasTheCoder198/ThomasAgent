package ragx

import (
	"crypto/ed25519"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
)

// droppingServer answers the first request on each connection and closes the connection on the next one without
// answering: to core this is the race behind R-EOF, where ragx closes a kept-alive connection core just reused.
type droppingServer struct {
	mu      sync.Mutex
	perConn map[string]int
	served  []*http.Request
	dropped int
}

func newDroppingServer(t *testing.T) (*droppingServer, *httptest.Server) {
	t.Helper()
	ds := &droppingServer{perConn: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ds.mu.Lock()
		ds.perConn[r.RemoteAddr]++
		drop := ds.perConn[r.RemoteAddr] > 1
		if drop {
			ds.dropped++
		} else {
			ds.served = append(ds.served, r)
		}
		ds.mu.Unlock()
		if drop {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err) // not require: FailNow must not run on the server's goroutine
				return
			}
			_ = conn.Close()
			return
		}
		writeJSON(w, http.StatusOK, `{"data":{},"meta":{"requestId":"x"}}`)
	}))
	t.Cleanup(server.Close)
	return ds, server
}

func droppingCaller(t *testing.T) (*Caller, *droppingServer) {
	t.Helper()
	ds, server := newDroppingServer(t)
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	caller, _, _ := callerAt(t, server.URL, key)
	// Warm the pool: the next call reuses this connection, and the server drops it.
	_, err = caller.Forward(orgContext(), readRequest())
	require.NoError(t, err)
	return caller, ds
}

func writeRequest(method, path, idempotencyKey string) ForwardRequest {
	return ForwardRequest{Method: method, Path: path, Route: testRoute, ContentType: "application/json",
		IdempotencyKey: idempotencyKey, Body: []byte(`{}`), Scope: apptoken.ScopeKBWrite}
}

func TestForward_RepeatsASafeCallWhenRagxDroppedTheReusedConnection(t *testing.T) {
	for name, path := range map[string]string{
		"confirm (idempotent by fileId, ruling P10)": "knowledge-bases/kb-1/files/file-1/confirm",
		"tool call": "tools/kb_search",
	} {
		t.Run(name, func(t *testing.T) {
			caller, ds := droppingCaller(t)
			_, err := caller.Forward(orgContext(), writeRequest(http.MethodPost, path, ""))
			require.NoError(t, err)
			require.Equal(t, 1, ds.dropped)
			last := ds.served[len(ds.served)-1]
			require.Equal(t, "/v1/"+path, last.URL.Path)
			require.Empty(t, last.Header.Values("Idempotency-Key"), "the marker is never sent on the wire")
		})
	}
}

func TestForward_NeverRepeatsCreatePresignOrDelete(t *testing.T) {
	for name, in := range map[string]ForwardRequest{
		"create KB":                   writeRequest(http.MethodPost, "knowledge-bases", ""),
		"presign":                     writeRequest(http.MethodPost, "knowledge-bases/kb-1/uploads", ""),
		"delete file":                 writeRequest(http.MethodDelete, "knowledge-bases/kb-1/files/file-1", ""),
		"confirm-like path elsewhere": writeRequest(http.MethodPost, "knowledge-bases/kb-1/files/file-1/confirm/x", ""),
	} {
		t.Run(name, func(t *testing.T) {
			caller, ds := droppingCaller(t)
			_, err := caller.Forward(orgContext(), in)
			require.ErrorIs(t, err, errors.ErrRagxUnavailable)
			require.Equal(t, 1, ds.dropped)
			require.Len(t, ds.served, 1, "only the warm-up call was answered")
		})
	}
}

// ragx dedupes by Idempotency-Key only on the file confirm route, so that is where the browser's key matters.
func TestForward_ResendsAConfirmWithTheBrowsersIdempotencyKeyKept(t *testing.T) {
	caller, ds := droppingCaller(t)
	_, err := caller.Forward(orgContext(),
		writeRequest(http.MethodPost, "knowledge-bases/kb-1/files/file-1/confirm", "idem-key-0001"))
	require.NoError(t, err)
	require.Equal(t, 1, ds.dropped)
	require.Equal(t, "idem-key-0001", ds.served[len(ds.served)-1].Header.Get("Idempotency-Key"))
}

func TestCaller_ClosesIdleRagxConnectionsAfterTheConfiguredTimeout(t *testing.T) {
	const idle = 50 * time.Millisecond
	var mu sync.Mutex
	opened, closed := 0, 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":{},"meta":{"requestId":"x"}}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch state {
		case http.StateNew:
			opened++
		case http.StateClosed:
			closed++
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	cfg := testConfig(server.URL)
	cfg.IdleConnTimeout = idle
	caller, _, _ := callerWith(t, cfg, key, discardToolLog{})

	_, err = caller.Forward(orgContext(), readRequest())
	require.NoError(t, err)
	// Core, not the server, closes the pooled connection once it has been idle for IdleConnTimeout.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return closed == 1
	}, 100*idle, idle/5) // generous: a loaded host is slow to run the idle timer
	_, err = caller.Forward(orgContext(), readRequest())
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, opened, "the second call dials a fresh connection")
}
