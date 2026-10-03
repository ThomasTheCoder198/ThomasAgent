package httpx

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const (
	requestIDPrefix  = "req_"
	panicCauseFormat = "panic: %v"
)

type Middleware = func(http.Handler) http.Handler

// Middlewares are registered before any route because chi panics on Use after routing starts.
func NewRouter(errorLogger ErrorLogger, middlewares ...Middleware) chi.Router {
	r := chi.NewRouter()
	r.Use(assignRequestID)
	r.Use(middlewares...)
	r.Use(injectErrorLogger(errorLogger), recoverPanics)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, errors.ErrNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, errors.ErrMethodNotAllowed)
	})
	return r
}

func assignRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = requestIDPrefix + uuid.NewString()
		}
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(logging.ContextWithRequestID(r.Context(), id)))
	})
}

func injectErrorLogger(errorLogger ErrorLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withErrorLogger(r.Context(), errorLogger)))
		})
	}
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler { //nolint:errorlint // net/http named error compared as net/http documents
					panic(rec)
				}
				WriteError(w, r, errors.ErrInternalError.WithCause(fmt.Errorf(panicCauseFormat, rec)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
