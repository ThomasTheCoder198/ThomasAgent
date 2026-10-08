package util

import (
	"net/http"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
)

// The browser holds two cookies: the HttpOnly session token and a readable CSRF token
// that the frontend echoes in CSRFHeader on unsafe methods.
const (
	SessionCookie = "thomas_session"
	CSRFCookie    = "thomas_csrf"
	CSRFHeader    = "X-CSRF-Token"
	cookiePath    = "/"
)

func SetSessionCookies(res http.ResponseWriter, session model.IssuedSession, secure bool) {
	http.SetCookie(res, &http.Cookie{
		Name:     SessionCookie,
		Value:    session.Token,
		Path:     cookiePath,
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(res, &http.Cookie{
		Name:     CSRFCookie,
		Value:    session.CSRFToken,
		Path:     cookiePath,
		Expires:  session.ExpiresAt,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies expires both cookies, so the frontend proxy sees no session and redirects to login.
func ClearSessionCookies(res http.ResponseWriter, secure bool) {
	for _, name := range []string{SessionCookie, CSRFCookie} {
		http.SetCookie(res, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     cookiePath,
			MaxAge:   -1,
			HttpOnly: name == SessionCookie,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}
