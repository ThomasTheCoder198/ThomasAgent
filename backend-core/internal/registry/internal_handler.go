package registry

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

func requireServiceToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			got := strings.TrimPrefix(header, bearerPrefix)
			if token == "" || !strings.HasPrefix(header, bearerPrefix) || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				httpx.WriteError(w, r, errors.New(errors.CodeInternalTokenInvalid))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func MountInternal(r chi.Router, svc *Service, serviceToken string) {
	r.Group(func(ir chi.Router) {
		ir.Use(requireServiceToken(serviceToken))
		ir.Get("/internal/models/resolve", func(w http.ResponseWriter, req *http.Request) {
			out, err := svc.Resolve(req.Context(), Role(req.URL.Query().Get("role")))
			respond(w, req, http.StatusOK, out, err)
		})
	})
}
