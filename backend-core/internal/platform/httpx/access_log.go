package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	accessLogMessage         = "http request"
	unmatchedRoute           = "unmatched"
	headerIdempotencyKey     = "Idempotency-Key"
	fieldMethod              = "method"
	fieldRoute               = "route"
	fieldStatus              = "status"
	fieldDurationMS          = "duration_ms"
	fieldIdempotencyKey      = "idempotency_key"
	fieldIdempotencyRejected = "idempotency_key_rejected"
	fieldRequestIDSource     = "request_id_source"
	fieldClientIDRejected    = "client_request_id_rejected"
	requestIDSourceGenerated = "generated"
)

// Idempotency keys are client text; only a bounded token alphabet reaches the logs.
var validIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type requestIDOrigin int

const (
	requestIDFromClient requestIDOrigin = iota
	requestIDGenerated
	requestIDReplaced
)

type requestIDOriginKey struct{}

func withRequestIDOrigin(ctx context.Context, origin requestIDOrigin) context.Context {
	return context.WithValue(ctx, requestIDOriginKey{}, origin)
}

func requestIDOriginFrom(ctx context.Context) requestIDOrigin {
	origin, _ := ctx.Value(requestIDOriginKey{}).(requestIDOrigin)
	return origin
}

// LogAccess writes one line per request when it finishes: 2xx/3xx info, 4xx warn, 5xx error.
func LogAccess(l *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			// Deferred so a connection aborted mid-stream (http.ErrAbortHandler) still gets an access-log line.
			defer func() {
				l.LogAttrs(r.Context(), accessLevel(rec.status), accessLogMessage, accessAttrs(r, rec.status, time.Since(start))...)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func accessLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func accessAttrs(r *http.Request, status int, elapsed time.Duration) []slog.Attr {
	attrs := []slog.Attr{
		slog.String(fieldMethod, r.Method),
		slog.String(fieldRoute, RoutePattern(r)),
		slog.Int(fieldStatus, status),
		slog.Int64(fieldDurationMS, elapsed.Milliseconds()),
	}
	switch requestIDOriginFrom(r.Context()) {
	case requestIDGenerated:
		attrs = append(attrs, slog.String(fieldRequestIDSource, requestIDSourceGenerated))
	case requestIDReplaced:
		attrs = append(attrs, slog.String(fieldRequestIDSource, requestIDSourceGenerated), slog.Bool(fieldClientIDRejected, true))
	}
	if key := r.Header.Get(headerIdempotencyKey); key != "" {
		if validIdempotencyKey.MatchString(key) {
			attrs = append(attrs, slog.String(fieldIdempotencyKey, key))
		} else {
			attrs = append(attrs, slog.Bool(fieldIdempotencyRejected, true))
		}
	}
	return attrs
}

// RoutePattern returns the matched chi pattern (for example /api/v1/providers/{id}), never the raw URL, so logs
// and metric labels stay low-cardinality. Call it after the router has served the request.
func RoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return unmatchedRoute
}
