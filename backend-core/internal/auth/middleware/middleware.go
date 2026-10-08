package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	CookieSession = "thomas_session"
	CookieCSRF    = "thomas_csrf"
	HeaderCSRF    = "X-CSRF-Token"
)

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

type userKey struct{}

func UserFrom(ctx context.Context) (model.User, bool) {
	u, ok := ctx.Value(userKey{}).(model.User)
	return u, ok
}

func RequireSession(svc service.SessionAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request
		c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
		cookie, err := r.Cookie(CookieSession)
		if err != nil {
			httpx.Fail(c, errors.ErrUnauthenticated)
			return
		}
		user, sess, err := svc.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !isSafeMethod(r.Method) && subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderCSRF)), []byte(sess.CSRFToken)) != 1 {
			httpx.Fail(c, errors.ErrAuthCsrfInvalid)
			return
		}
		ctx := tenant.WithID(r.Context(), sess.TenantID)
		c.Request = r.WithContext(context.WithValue(ctx, userKey{}, user))
		c.Next()
	}
}
