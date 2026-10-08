package auth

import "net/http"

const cookiePath = "/"

// setSessionCookies stores the raw session token HttpOnly and the CSRF token readable by the frontend.
func setSessionCookies(res http.ResponseWriter, session IssuedSession, secure bool) {
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

// clearSessionCookies expires both cookies, so the frontend proxy sees no session and redirects to login.
func clearSessionCookies(res http.ResponseWriter, secure bool) {
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
