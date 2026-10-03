package httpx

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apperr"
)

type requestIDKey struct{}

const (
	requestIDPrefix  = "req_"
	panicCauseFormat = "panic: %v"
)

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type Middleware = func(http.Handler) http.Handler

// Middlewares are registered before any route because chi panics on Use after routing starts.
func NewRouter(logErr ErrorLogger, mw ...Middleware) chi.Router {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(mw...)
	r.Use(errorLogging(logErr), recoverer)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, apperr.New(apperr.CodeNotFound))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, apperr.New(apperr.CodeMethodNotAllowed))
	})
	return r
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = requestIDPrefix + uuid.NewString()
		}
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func errorLogging(logErr ErrorLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withErrorLogger(r.Context(), logErr)))
		})
	}
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel compared as net/http documents
					panic(rec)
				}
				WriteError(w, r, apperr.New(apperr.CodeInternalError, apperr.WithCause(fmt.Errorf(panicCauseFormat, rec))))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
