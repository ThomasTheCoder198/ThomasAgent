package httpx

import (
	"net/http"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

// HTTPMetrics is the part of the metrics registry the router needs.
type HTTPMetrics interface {
	ObserveHTTP(method, route string, status int, elapsed time.Duration, requestID string)
}

// RecordMetrics observes every request after routing, so the route label is the chi pattern. Nil disables it.
func RecordMetrics(rec HTTPMetrics) Middleware {
	return func(next http.Handler) http.Handler {
		if rec == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			status := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				rec.ObserveHTTP(r.Method, RoutePattern(r), status.status, time.Since(start), logging.RequestIDFromContext(r.Context()))
			}()
			next.ServeHTTP(status, r)
		})
	}
}
