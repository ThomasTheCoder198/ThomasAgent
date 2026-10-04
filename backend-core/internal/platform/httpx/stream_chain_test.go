package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	chainHeartbeat     = 20 * time.Millisecond
	chainWaitBound     = 5 * time.Second
	chainSessionHeader = "X-Test-Session"
	chainCSRFHeader    = "X-Test-CSRF"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// requireHeader stands in for the session and CSRF middleware that Task 4 adds, so this gate runs the same chain shape.
func requireHeader(name string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(name) == "" {
				WriteError(w, r, errors.ErrUnauthenticated)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newChainServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *lockedBuffer) {
	t.Helper()
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	r := NewRouter(NewSlogErrorLogger(logger), TraceRequests("chain-test"), LogAccess(logger),
		requireHeader(chainSessionHeader), requireHeader(chainCSRFHeader))
	r.Post("/stream", handler)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	return server, logs
}

func openStream(ctx context.Context, t *testing.T, server *httptest.Server) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/stream", nil)
	require.NoError(t, err)
	req.Header.Set(chainSessionHeader, "session")
	req.Header.Set(chainCSRFHeader, "csrf")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// readFrame returns one SSE frame (the lines up to the blank line).
func readFrame(reader *bufio.Reader) (string, error) {
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return frame.String(), err
		}
		if line == "\n" {
			return frame.String(), nil
		}
		frame.WriteString(line)
	}
}

func accessLogEntries(logs *lockedBuffer) []map[string]any {
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "http request" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestStreamChain_DeliversFramesIncrementally(t *testing.T) {
	release := make(chan struct{})
	server, _ := newChainServer(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, time.Hour)
		if err != nil {
			return
		}
		defer stream.Close()
		_ = stream.Send([]byte(`{"n":1}`))
		select {
		case <-release:
		case <-stream.Done():
			return
		}
		_ = stream.Send([]byte(`{"n":2}`))
	})
	ctx, cancel := context.WithTimeout(t.Context(), chainWaitBound) // a buffering bug becomes an error, not a hang
	defer cancel()
	resp := openStream(ctx, t, server)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	reader := bufio.NewReader(resp.Body)

	first, err := readFrame(reader) // the handler is still parked on release
	require.NoError(t, err)
	require.Equal(t, "data: {\"n\":1}\n", first)
	close(release)
	second, err := readFrame(reader)
	require.NoError(t, err)
	require.Equal(t, "data: {\"n\":2}\n", second)
	_, err = io.ReadAll(reader)
	require.NoError(t, err)
}

func TestStreamChain_AccessLogRecordsStreamOnce(t *testing.T) {
	server, logs := newChainServer(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, time.Hour)
		if err != nil {
			return
		}
		defer stream.Close()
		_ = stream.Send([]byte(`{"n":1}`))
	})
	ctx, cancel := context.WithTimeout(t.Context(), chainWaitBound)
	defer cancel()
	resp := openStream(ctx, t, server)
	defer func() { _ = resp.Body.Close() }()
	_, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(accessLogEntries(logs)) == 1 }, chainWaitBound, time.Millisecond)
	require.EqualValues(t, http.StatusOK, accessLogEntries(logs)[0]["status"])
}

func TestStreamChain_ClientDisconnectCancelsHandler(t *testing.T) {
	cancelled := make(chan struct{})
	server, _ := newChainServer(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, time.Hour)
		if err != nil {
			return
		}
		defer stream.Close()
		_ = stream.Send([]byte(`{"n":1}`))
		<-stream.Done()
		close(cancelled)
	})
	ctx, cancel := context.WithCancel(t.Context())
	resp := openStream(ctx, t, server)
	defer func() { _ = resp.Body.Close() }()
	_, err := readFrame(bufio.NewReader(resp.Body))
	require.NoError(t, err)
	cancel()
	select {
	case <-cancelled:
	case <-time.After(chainWaitBound):
		t.Fatal("handler context was not cancelled after the client disconnected")
	}
}

func TestStreamChain_PanicAfterFirstFrameAbortsWithoutJSON(t *testing.T) {
	server, logs := newChainServer(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, time.Hour)
		if err != nil {
			return
		}
		defer stream.Close()
		_ = stream.Send([]byte(`{"n":1}`))
		panic("secret internal state")
	})
	ctx, cancel := context.WithTimeout(t.Context(), chainWaitBound)
	defer cancel()
	resp := openStream(ctx, t, server)
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	first, err := readFrame(reader)
	require.NoError(t, err)
	require.Equal(t, "data: {\"n\":1}\n", first)
	rest, err := io.ReadAll(reader)
	require.Error(t, err, "the connection is aborted, not ended cleanly")
	require.NotContains(t, string(rest), "INTERNAL_ERROR")
	require.NotContains(t, string(rest), "secret internal state")
	require.Eventually(t, func() bool { return strings.Count(logs.String(), `"msg":"request failed"`) == 1 }, chainWaitBound, time.Millisecond)
	require.Eventually(t, func() bool { return len(accessLogEntries(logs)) == 1 }, chainWaitBound, time.Millisecond)
}

func TestStreamChain_IdleStreamSendsHeartbeats(t *testing.T) {
	server, _ := newChainServer(t, func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, chainHeartbeat)
		if err != nil {
			return
		}
		defer stream.Close()
		<-stream.Done()
	})
	ctx, cancel := context.WithTimeout(t.Context(), chainWaitBound)
	defer cancel()
	resp := openStream(ctx, t, server)
	defer func() { _ = resp.Body.Close() }()
	frame, err := readFrame(bufio.NewReader(resp.Body))
	require.NoError(t, err)
	require.Equal(t, ": ping\n", frame)
}
