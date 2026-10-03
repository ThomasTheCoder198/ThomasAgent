package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TraceRequests(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return otelhttp.NewHandler(next, serviceName) }
}

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

// SSE handlers (M2) need http.Flusher; Unwrap lets http.ResponseController reach the original writer.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		if !s.committed {
			s.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func LogAccess(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			l.InfoContext(r.Context(), "http request",
				"method", r.Method, "path", r.URL.Path, "status", rec.status,
				"duration_ms", time.Since(start).Milliseconds())
		})
	}
}

func NewSlogErrorLogger(l *slog.Logger) ErrorLogger {
	return func(ctx context.Context, err *errors.AppError) {
		l.ErrorContext(ctx, "request failed", "code", string(err.Code), "error", err.Error(), "details", err.Details)
	}
}
