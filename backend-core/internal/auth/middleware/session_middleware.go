package middleware

import (
	"context"
	"crypto/subtle"
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/util"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type userContextKey struct{}

// UserFromContext returns the user that RequireSession authenticated for this request.
func UserFromContext(ctx context.Context) (model.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(model.User)
	return user, ok
}

// WithUser carries an already authenticated user into downstream work and tests.
func WithUser(ctx context.Context, user model.User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// RequireSession authenticates the session cookie, checks the CSRF header on unsafe methods,
// and puts the session's tenant and user into the request context. A cookie that no longer
// matches a live session (expired, purged or logged out elsewhere) is cleared in the response.
func RequireSession(authService service.AuthService, cookieSecure bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		req := ctx.Request
		ctx.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
		cookie, err := req.Cookie(util.SessionCookie)
		if err != nil {
			httpx.Fail(ctx, errors.ErrUnauthenticated)
			return
		}
		user, session, err := authService.Authenticate(req.Context(), cookie.Value)
		if err != nil {
			if isStaleSession(err) {
				util.ClearSessionCookies(ctx.Writer, cookieSecure)
			}
			httpx.Fail(ctx, err)
			return
		}
		if !isSafeMethod(req.Method) && !csrfTokenMatches(req.Header.Get(util.CSRFHeader), session.CSRFToken) {
			httpx.Fail(ctx, errors.ErrAuthCsrfInvalid)
			return
		}
		requestContext := tenant.WithID(req.Context(), session.TenantID)
		ctx.Request = req.WithContext(WithUser(requestContext, user))
		ctx.Next()
	}
}

func isStaleSession(err error) bool {
	return stderrors.Is(err, errors.ErrAuthSessionExpired) || stderrors.Is(err, errors.ErrUnauthenticated)
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func csrfTokenMatches(sent, stored string) bool {
	return subtle.ConstantTimeCompare([]byte(sent), []byte(stored)) == 1
}
