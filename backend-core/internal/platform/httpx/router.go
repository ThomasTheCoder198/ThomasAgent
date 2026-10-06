package httpx

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const panicCauseFormat = "panic: %v"

type Middleware = func(http.Handler) http.Handler

// Middlewares are registered before any route because chi panics on Use after routing starts.
func NewRouter(errorLogger ErrorLogger, middlewares ...Middleware) chi.Router {
	r := chi.NewRouter()
	r.Use(assignRequestID)
	r.Use(middlewares...)
	r.Use(injectErrorLogger(errorLogger), trackCommit, recoverPanics)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, errors.ErrNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		WriteError(w, req, errors.ErrMethodNotAllowed)
	})
	return r
}

// assignRequestID trusts a client id only when it matches the shared pattern, so a forged header cannot inject
// arbitrary text into every log line of the request.
func assignRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, origin := acceptRequestID(r.Header.Get(requestid.Header))
		w.Header().Set(requestid.Header, id)
		ctx := withRequestIDOrigin(logging.ContextWithRequestID(r.Context(), id), origin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func acceptRequestID(sent string) (string, requestIDOrigin) {
	switch {
	case requestid.IsValid(sent):
		return sent, requestIDFromClient
	case sent == "":
		return requestid.NewRequest(), requestIDGenerated
	default:
		return requestid.NewRequest(), requestIDReplaced
	}
}

func injectErrorLogger(errorLogger ErrorLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withErrorLogger(r.Context(), errorLogger)))
		})
	}
}

// trackCommit gives handlers a writer that records whether the response was committed.
func trackCommit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&statusRecorder{ResponseWriter: w, status: http.StatusOK}, r)
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				handlePanic(w, r, rec)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func handlePanic(w http.ResponseWriter, r *http.Request, rec any) {
	if rec == http.ErrAbortHandler { //nolint:errorlint // net/http named error compared as net/http documents
		panic(rec)
	}
	panicErr := errors.ErrInternalError.WithCause(fmt.Errorf(panicCauseFormat, rec))
	if ResponseCommitted(w) {
		// A partial response is already on the wire: log, then abort the connection instead of appending JSON.
		logBoundaryError(r, panicErr)
		panic(http.ErrAbortHandler)
	}
	WriteError(w, r, panicErr)
}
