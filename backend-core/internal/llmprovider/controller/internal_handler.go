package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/service"
)

func RequireServiceToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
			parts := strings.Fields(r.Header.Get(httpx.HeaderAuthorization))
			if token == "" || len(parts) != 2 || !strings.EqualFold(parts[0], httpx.AuthSchemeBearer) {
				httpx.WriteError(w, r, errors.ErrInternalTokenInvalid)
				return
			}
			expected := sha256.Sum256([]byte(token))
			supplied := sha256.Sum256([]byte(parts[1]))
			if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
				httpx.WriteError(w, r, errors.ErrInternalTokenInvalid)
				return
			}
			next.ServeHTTP(w, r.WithContext(tenant.WithID(r.Context(), tenant.PlatformID)))
		})
	}
}

func MountInternal(r chi.Router, svc service.Resolver, serviceToken string) {
	r.Group(func(ir chi.Router) {
		ir.Use(RequireServiceToken(serviceToken))
		ir.Get("/internal/models/resolve", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
			out, err := svc.Resolve(req.Context(), model.Role(req.URL.Query().Get("role")))
			respond(w, req, http.StatusOK, dto.FromResolvedModel(out), err)
		})
	})
}
