package httpx

import (
	"bytes"
	stderrors "errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

func TestEventStreamBoundsWriteToConnectedNonReadingClient(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close() // Keep the peer connected without reading any stream bytes.
	writer := pipeResponseWriter{conn: serverSide, header: make(http.Header)}
	stream, err := NewEventStream(writer, httptest.NewRequest(http.MethodGet, "/", nil), config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: 50 * time.Millisecond})
	require.NoError(t, err)
	started := time.Now()
	err = stream.Send(bytes.Repeat([]byte{'x'}, 1<<20))
	select {
	case <-stream.Done():
	default:
		t.Error("write failure must signal the producer to stop")
	}
	stream.Close()
	require.Error(t, err)
	require.Less(t, time.Since(started), time.Second)
}

func TestEventStreamBoundsInitialHeaderFlush(t *testing.T) {
	writer := &deadlineHeaderWriter{safeRecorder: newSafeRecorder()}
	stream, err := NewEventStream(writer, httptest.NewRequest(http.MethodGet, "/", nil), config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
	require.NoError(t, err)
	defer stream.Close()
	require.True(t, writer.boundedFlush, "the initial headers need the same bound as data frames")
}

type deadlineHeaderWriter struct {
	*safeRecorder
	deadline     time.Time
	boundedFlush bool
}

func (w *deadlineHeaderWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}
func (w *deadlineHeaderWriter) Flush() { w.boundedFlush = !w.deadline.IsZero(); w.safeRecorder.Flush() }

type pipeResponseWriter struct {
	conn   net.Conn
	header http.Header
}

func (w pipeResponseWriter) Header() http.Header         { return w.header }
func (w pipeResponseWriter) WriteHeader(int)             {}
func (w pipeResponseWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }
func (w pipeResponseWriter) Flush()                      {}
func (w pipeResponseWriter) SetWriteDeadline(deadline time.Time) error {
	return w.conn.SetWriteDeadline(deadline)
}

var errTestFlush = stderrors.New("test flush failure")

// noFlushWriter deliberately hides the recorder's optional flushing capabilities.
type noFlushWriter struct{ recorder *httptest.ResponseRecorder }

func (w *noFlushWriter) Header() http.Header              { return w.recorder.Header() }
func (w *noFlushWriter) SetWriteDeadline(time.Time) error { return nil }
func (w *noFlushWriter) WriteHeader(code int)             { w.recorder.WriteHeader(code) }
func (w *noFlushWriter) Write(p []byte) (int, error)      { return w.recorder.Write(p) }

type errorFlushWriter struct {
	noFlushWriter
	flushErr error
	flushes  int
}

func (w *errorFlushWriter) FlushError() error { w.flushes++; return w.flushErr }

func TestEventStream_RouterRejectsUnsupportedFlush(t *testing.T) {
	writer := &noFlushWriter{recorder: httptest.NewRecorder()}
	router := NewRouter(nil)
	router.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
		if stream != nil {
			stream.Close()
		}
		require.ErrorIs(t, err, http.ErrNotSupported)
		require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
	})
	router.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
}

func TestEventStream_RouterForwardsFlushError(t *testing.T) {
	writer := &errorFlushWriter{noFlushWriter: noFlushWriter{recorder: httptest.NewRecorder()}, flushErr: errTestFlush}
	router := NewRouter(nil)
	router.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
		if stream != nil {
			stream.Close()
		}
		require.ErrorIs(t, err, errTestFlush)
		require.Equal(t, errors.CodeInternalError, errors.ToAppError(err).Code)
	})
	router.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
	require.Equal(t, 1, writer.flushes)
}

func TestEventStream_RouterForwardsFlushErrorOnlySuccessAndSendFailure(t *testing.T) {
	writer := &errorFlushWriter{noFlushWriter: noFlushWriter{recorder: httptest.NewRecorder()}}
	router := NewRouter(nil)
	router.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
		require.NoError(t, err)
		defer stream.Close()
		require.Equal(t, 1, writer.flushes)
		require.NoError(t, stream.Send([]byte("first")))
		require.Equal(t, 2, writer.flushes)
		writer.flushErr = errTestFlush
		require.ErrorIs(t, stream.Send([]byte("second")), errTestFlush)
		require.Equal(t, 3, writer.flushes)
	})
	router.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
}

func TestStatusRecorder_ErrorFlushCommitsImplicitStatus(t *testing.T) {
	writer := &errorFlushWriter{noFlushWriter: noFlushWriter{recorder: httptest.NewRecorder()}}
	recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
	require.NoError(t, http.NewResponseController(recorder).Flush())
	require.True(t, ResponseCommitted(recorder))
	require.Equal(t, http.StatusOK, writer.recorder.Code)
	require.Equal(t, 1, writer.flushes)
}

type unwrapFlushWriter struct{ http.ResponseWriter }

func (w *unwrapFlushWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestEventStream_RouterFlushesThroughNestedUnwrap(t *testing.T) {
	writer := &errorFlushWriter{noFlushWriter: noFlushWriter{recorder: httptest.NewRecorder()}}
	wrapped := &unwrapFlushWriter{ResponseWriter: &unwrapFlushWriter{ResponseWriter: writer}}
	router := NewRouter(nil)
	router.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := NewEventStream(w, r, config.HTTPConfig{EventStreamHeartbeat: time.Hour, EventStreamWriteTimeout: time.Second})
		require.NoError(t, err)
		defer stream.Close()
		require.Equal(t, 1, writer.flushes)
		require.NoError(t, stream.Send([]byte("first")))
		require.Equal(t, 2, writer.flushes)
	})
	router.ServeHTTP(wrapped, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
}
