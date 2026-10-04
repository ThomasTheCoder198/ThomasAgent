package httpx

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

func TestLimitRequestDuration_SetsDeadlineOnlyWhereApplied(t *testing.T) {
	const limit = 2 * time.Second
	var limited, streaming bool
	r := NewRouter(func(context.Context, *errors.AppError) {})
	r.Group(func(g chi.Router) {
		g.Use(LimitRequestDuration(limit))
		g.Get("/api", func(_ http.ResponseWriter, req *http.Request) {
			deadline, ok := req.Context().Deadline()
			limited = ok && time.Until(deadline) <= limit
		})
	})
	r.Get("/stream", func(_ http.ResponseWriter, req *http.Request) {
		_, ok := req.Context().Deadline()
		streaming = !ok
	})
	performRequest(r, http.MethodGet, "/api", "vi")
	performRequest(r, http.MethodGet, "/stream", "vi")
	require.True(t, limited, "ordinary routes carry the deadline")
	require.True(t, streaming, "stream routes must not")
}
