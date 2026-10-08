package httpx

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// ResponseCommitted reports whether the status line already went to the client. After that point a JSON error body
// would corrupt the response (it is typically an SSE stream), so callers must only log.
func ResponseCommitted(w http.ResponseWriter) bool {
	for w != nil {
		if writer, ok := w.(interface{ Written() bool }); ok {
			return writer.Written()
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
