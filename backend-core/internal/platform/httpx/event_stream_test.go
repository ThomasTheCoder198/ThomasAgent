package httpx

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"

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

func (s *safeRecorder) Header() http.Header              { return s.header }
func (s *safeRecorder) SetWriteDeadline(time.Time) error { return nil }
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
	stream, err := NewEventStream(rec, req, config.HTTPConfig{EventStreamHeartbeat: heartbeat, EventStreamWriteTimeout: time.Second})
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
	stream, err := NewEventStream(rec, httptest.NewRequestWithContext(ctx, http.MethodPost, "/", nil), config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
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
	_, err := NewEventStream(newSafeRecorder(), req, config.HTTPConfig{EventStreamWriteTimeout: time.Second})
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

type wrappedStreamWriter struct{ recorder *safeRecorder }

func (w wrappedStreamWriter) Header() http.Header            { return w.recorder.Header() }
func (w wrappedStreamWriter) WriteHeader(code int)           { w.recorder.WriteHeader(code) }
func (w wrappedStreamWriter) Write(data []byte) (int, error) { return w.recorder.Write(data) }
func (w wrappedStreamWriter) Flush()                         { w.recorder.Flush() }

type unsupportedDeadlineWriter struct{ wrappedStreamWriter }

func (w unsupportedDeadlineWriter) SetWriteDeadline(time.Time) error {
	return fmt.Errorf("wrapper deadline: %w", http.ErrNotSupported)
}

func TestEventStream_WrappedWriterWithoutDeadlineStillStreams(t *testing.T) {
	for _, wrappedError := range []bool{false, true} {
		t.Run(fmt.Sprint(wrappedError), func(t *testing.T) {
			recorder := newSafeRecorder()
			var writer http.ResponseWriter = wrappedStreamWriter{recorder: recorder}
			if wrappedError {
				writer = unsupportedDeadlineWriter{wrappedStreamWriter{recorder: recorder}}
			}
			stream, err := NewEventStream(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
			require.NoError(t, err)
			defer stream.Close()
			require.NoError(t, stream.Send([]byte("ready")))
			require.Equal(t, "data: ready\n\n", recorder.Body())
		})
	}
}

type stalledHeartbeatWriter struct {
	*safeRecorder
	entered chan struct{}
	release chan struct{}
}

func (w *stalledHeartbeatWriter) Write(data []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.safeRecorder.Write(data)
}

func TestEventStream_CancellationSignalsDoneDuringBlockedHeartbeat(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := &stalledHeartbeatWriter{safeRecorder: newSafeRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	stream, err := NewEventStream(writer, httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil), config.HTTPConfig{EventStreamHeartbeat: time.Millisecond, EventStreamWriteTimeout: time.Second})
	require.NoError(t, err)
	defer stream.Close()
	defer close(writer.release)
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not start")
	}
	cancel()
	select {
	case <-stream.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done must signal cancellation without waiting for a blocked heartbeat write")
	}
}
