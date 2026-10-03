package httpx

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func MountHealth(r chi.Router, deps map[string]Pinger) {
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		WriteData(w, req, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		failing := map[string]any{}
		for name, dep := range deps {
			if err := dep.Ping(req.Context()); err != nil {
				failing[name] = "unreachable"
			}
		}
		if len(failing) > 0 {
			WriteError(w, req, apperr.New(apperr.CodeProviderUnavailable, apperr.WithDetails(failing)))
			return
		}
		WriteData(w, req, http.StatusOK, map[string]string{"status": "ready"})
	})
}
