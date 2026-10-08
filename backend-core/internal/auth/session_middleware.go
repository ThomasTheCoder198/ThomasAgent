package auth

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type sessionAuthenticator interface {
	Authenticate(ctx context.Context, sessionToken string) (User, Session, error)
}

type userContextKey struct{}

// UserFromContext returns the user that RequireSession authenticated for this request.
func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

// RequireSession authenticates the session cookie, checks the CSRF header on unsafe methods,
// and puts the session's tenant and user into the request context.
func RequireSession(authenticator sessionAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := c.Request
		c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
		cookie, err := req.Cookie(SessionCookie)
		if err != nil {
			httpx.Fail(c, errors.ErrUnauthenticated)
			return
		}
		user, session, err := authenticator.Authenticate(req.Context(), cookie.Value)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !isSafeMethod(req.Method) && !csrfTokenMatches(req.Header.Get(CSRFHeader), session.CSRFToken) {
			httpx.Fail(c, errors.ErrAuthCsrfInvalid)
			return
		}
		ctx := tenant.WithID(req.Context(), session.TenantID)
		c.Request = req.WithContext(context.WithValue(ctx, userContextKey{}, user))
		c.Next()
	}
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func csrfTokenMatches(sent, stored string) bool {
	return subtle.ConstantTimeCompare([]byte(sent), []byte(stored)) == 1
}
