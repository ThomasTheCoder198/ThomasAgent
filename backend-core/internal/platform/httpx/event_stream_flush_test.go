package httpx

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

var errTestFlush = stderrors.New("test flush failure")

// noFlushWriter deliberately hides the recorder's optional flushing capabilities.
type noFlushWriter struct{ recorder *httptest.ResponseRecorder }

func (w *noFlushWriter) Header() http.Header         { return w.recorder.Header() }
func (w *noFlushWriter) WriteHeader(code int)        { w.recorder.WriteHeader(code) }
func (w *noFlushWriter) Write(p []byte) (int, error) { return w.recorder.Write(p) }

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
		stream, err := NewEventStream(w, r, time.Hour)
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
		stream, err := NewEventStream(w, r, time.Hour)
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
		stream, err := NewEventStream(w, r, time.Hour)
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
		stream, err := NewEventStream(w, r, time.Hour)
		require.NoError(t, err)
		defer stream.Close()
		require.Equal(t, 1, writer.flushes)
		require.NoError(t, stream.Send([]byte("first")))
		require.Equal(t, 2, writer.flushes)
	})
	router.ServeHTTP(wrapped, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
}
