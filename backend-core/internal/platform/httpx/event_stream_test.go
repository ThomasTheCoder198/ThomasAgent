package httpx

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const (
	testHeartbeat  = 10 * time.Millisecond
	testWaitBound  = 5 * time.Second
	testPollPeriod = 5 * time.Millisecond
)

// safeRecorder is a goroutine-safe ResponseWriter: the heartbeat writes concurrently with the test reading.
type safeRecorder struct {
	mu      sync.Mutex
	header  http.Header
	status  int
	body    bytes.Buffer
	flushes int
}

func newSafeRecorder() *safeRecorder { return &safeRecorder{header: http.Header{}} }

func (s *safeRecorder) Header() http.Header { return s.header }
func (s *safeRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
}
func (s *safeRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body.Write(p)
}
func (s *safeRecorder) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushes++
}
func (s *safeRecorder) Body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body.String()
}

func newStream(t *testing.T, heartbeat time.Duration) (*EventStream, *safeRecorder) {
	t.Helper()
	rec := newSafeRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	stream, err := NewEventStream(rec, req, heartbeat)
	require.NoError(t, err)
	t.Cleanup(stream.Close)
	return stream, rec
}

func TestEventStream_CommitsStreamingHeaders(t *testing.T) {
	_, rec := newStream(t, time.Hour)
	require.Equal(t, http.StatusOK, rec.status)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no-cache, no-transform", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.Positive(t, rec.flushes)
}

func TestEventStream_SendWritesOneFlushedFrame(t *testing.T) {
	stream, rec := newStream(t, time.Hour)
	before := rec.flushes
	require.NoError(t, stream.Send([]byte(`{"n":1}`)))
	require.Equal(t, "data: {\"n\":1}\n\n", rec.Body())
	require.Greater(t, rec.flushes, before)
}

func TestEventStream_SendRejectsMultiLinePayload(t *testing.T) {
	stream, rec := newStream(t, time.Hour)
	require.Error(t, stream.Send([]byte("a\nb")))
	require.Empty(t, rec.Body())
}

func TestEventStream_HeartbeatWritesComments(t *testing.T) {
	_, rec := newStream(t, testHeartbeat)
	require.Eventually(t, func() bool { return bytes.Contains([]byte(rec.Body()), []byte(": ping\n\n")) }, testWaitBound, testPollPeriod)
}

func TestEventStream_SendAfterCloseFails(t *testing.T) {
	stream, _ := newStream(t, time.Hour)
	stream.Close()
	require.ErrorIs(t, stream.Send([]byte("x")), ErrEventStreamClosed)
}

func TestEventStream_DoneClosesWhenClientIsGone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rec := newSafeRecorder()
	stream, err := NewEventStream(rec, httptest.NewRequestWithContext(ctx, http.MethodPost, "/", nil), time.Hour)
	require.NoError(t, err)
	defer stream.Close()
	cancel()
	select {
	case <-stream.Done():
	case <-time.After(testWaitBound):
		t.Fatal("Done was not closed after the client went away")
	}
	require.ErrorIs(t, stream.Send([]byte("x")), ErrEventStreamClosed)
}

func TestEventStream_CloseStopsHeartbeat(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	stream, _ := newStream(t, testHeartbeat)
	stream.Close()
	goleak.VerifyNone(t, ignore)
}

func TestNewEventStream_RejectsNonPositiveHeartbeat(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	_, err := NewEventStream(newSafeRecorder(), req, 0)
	require.Error(t, err)
}

func TestEventStream_SendRejectsCarriageReturnPayload(t *testing.T) {
	for name, payload := range map[string]string{"bare CR": "a\rb", "injected frame": "a\r\rdata: injected"} {
		t.Run(name, func(t *testing.T) {
			stream, rec := newStream(t, time.Hour)
			require.Error(t, stream.Send([]byte(payload)))
			require.Empty(t, rec.Body())
		})
	}
}
