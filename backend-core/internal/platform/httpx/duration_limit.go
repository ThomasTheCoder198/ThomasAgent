package httpx

import (
	"context"
	"net/http"
	"time"
)

// LimitRequestDuration puts a deadline on the request context of ordinary routes. It must NOT wrap stream routes:
// their lifetime is the run deadline owned by the agent runtime (spec §7.1). It only sets the deadline; it does not
// buffer or wrap the writer, so Flush keeps working. http.Server deliberately has no WriteTimeout for the same reason.
func LimitRequestDuration(limit time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), limit)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
