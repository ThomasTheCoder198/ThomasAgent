package auth

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
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

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

func RequireSession(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(CookieSession)
			if err != nil {
				httpx.WriteError(w, r, errors.New(errors.CodeUnauthenticated))
				return
			}
			user, sess, err := svc.Authenticate(r.Context(), c.Value)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if !isSafeMethod(r.Method) && subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderCSRF)), []byte(sess.CSRFToken)) != 1 {
				httpx.WriteError(w, r, errors.New(errors.CodeAuthCsrfInvalid))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
		})
	}
}
