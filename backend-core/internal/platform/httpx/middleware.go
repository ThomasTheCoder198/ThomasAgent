package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

type statusRecorder struct {
	http.ResponseWriter
	status    int
	committed bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.committed {
		return
	}
	if code >= http.StatusOK {
		s.status = code
		s.committed = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(body []byte) (int, error) {
	if !s.committed {
		s.WriteHeader(http.StatusOK)
	}
	n, err := s.ResponseWriter.Write(body)
	if err != nil {
		return n, fmt.Errorf("write HTTP response: %w", err)
	}
	return n, nil
}

// Flush preserves http.Flusher compatibility; ResponseController uses FlushError to retain failures.
func (s *statusRecorder) Flush() { _ = s.FlushError() }

// Forward through ResponseController so optional FlushError and Unwrap capabilities are not hidden.
func (s *statusRecorder) FlushError() error {
	if !s.committed {
		s.WriteHeader(http.StatusOK)
	}
	if err := http.NewResponseController(s.ResponseWriter).Flush(); err != nil {
		return fmt.Errorf("flush HTTP response: %w", err)
	}
	return nil
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// ResponseCommitted reports whether the status line already went to the client. After that point a JSON error body
// would corrupt the response (it is typically an SSE stream), so callers must only log.
func ResponseCommitted(w http.ResponseWriter) bool {
	for w != nil {
		if rec, ok := w.(*statusRecorder); ok {
			return rec.committed
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = unwrapper.Unwrap()
	}
	return false
}

func NewSlogErrorLogger(l *slog.Logger) ErrorLogger {
	return func(ctx context.Context, err *errors.AppError) {
		l.ErrorContext(ctx, "request failed", "code", string(err.Code), "error", err.Error(), "details", err.Details)
	}
}
