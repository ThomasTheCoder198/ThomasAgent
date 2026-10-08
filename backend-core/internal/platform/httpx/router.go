package httpx

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const panicCauseFormat = "panic: %v"

type Middleware = gin.HandlerFunc

func NewRouter(errorLogger ErrorLogger, middlewares ...Middleware) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.Use(assignRequestID)
	r.Use(middlewares...)
	r.Use(injectErrorLogger(errorLogger), recoverPanics)
	r.NoRoute(func(c *gin.Context) { Fail(c, errors.ErrNotFound) })
	r.NoMethod(func(c *gin.Context) { Fail(c, errors.ErrMethodNotAllowed) })
	return r
}

func assignRequestID(c *gin.Context) {
	id, origin := acceptRequestID(c.Request.Header.Get(requestid.Header))
	c.Writer.Header().Set(requestid.Header, id)
	ctx := withRequestIDOrigin(logging.ContextWithRequestID(c.Request.Context(), id), origin)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
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

func injectErrorLogger(errorLogger ErrorLogger) Middleware {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(withErrorLogger(c.Request.Context(), errorLogger))
		c.Next()
	}
}

func recoverPanics(c *gin.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			handlePanic(c.Writer, c.Request, rec)
			c.Abort()
		}
	}()
	c.Next()
}

func handlePanic(w http.ResponseWriter, r *http.Request, rec any) {
	if rec == http.ErrAbortHandler { //nolint:errorlint // net/http documents this named panic value
		panic(rec)
	}
	panicErr := errors.ErrInternalError.WithCause(fmt.Errorf(panicCauseFormat, rec))
	if ResponseCommitted(w) {
		logBoundaryError(r, panicErr)
		panic(http.ErrAbortHandler)
	}
	WriteError(w, r, panicErr)
}
