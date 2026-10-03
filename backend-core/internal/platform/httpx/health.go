package httpx

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	LivenessPath                = "/healthz"
	ReadinessPath               = "/readyz"
	LivenessStatus              = "ok"
	ReadinessStatus             = "ready"
	DependencyUnreachableStatus = "unreachable"
)

type DependencyPinger interface {
	Ping(ctx context.Context) error
}

func MountHealth(r chi.Router, deps map[string]DependencyPinger) {
	r.Get(LivenessPath, func(w http.ResponseWriter, req *http.Request) {
		WriteSuccess(w, req, http.StatusOK, map[string]string{"status": LivenessStatus})
	})
	r.Get(ReadinessPath, func(w http.ResponseWriter, req *http.Request) {
		failing := map[string]any{}
		for name, dep := range deps {
			if err := dep.Ping(req.Context()); err != nil {
				failing[name] = DependencyUnreachableStatus
			}
		}
		if len(failing) > 0 {
			WriteError(w, req, errors.ErrProviderUnavailable.WithDetails(failing))
			return
		}
		WriteSuccess(w, req, http.StatusOK, map[string]string{"status": ReadinessStatus})
	})
}
