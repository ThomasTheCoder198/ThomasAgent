package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

func requireCookiesCleared(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for _, name := range []string{SessionCookie, CSRFCookie} {
		cleared := cookie(response, name)
		require.NotNil(t, cleared, name+" must be cleared")
		require.Empty(t, cleared.Value)
		require.Negative(t, cleared.MaxAge, name+" must expire immediately")
	}
}

func getMe(handler http.Handler, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRequireSession_ExpiredSessionClearsCookies(t *testing.T) {
	service, _ := newTestService(t)
	handler := httpx.NewRouter(nil)
	mountAuthForTest(handler, ControllerParams{Service: service, Limiter: newTestLimiter(startRedis(t), 10)})
	sessionCookie := cookie(login(t, handler, ownerEmail, ownerPassword), SessionCookie)
	_, err := service.db.Exec(t.Context(), `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, hashSessionToken(sessionCookie.Value))
	require.NoError(t, err)

	response := getMe(handler, sessionCookie)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, "AUTH_SESSION_EXPIRED", errCode(t, response))
	requireCookiesCleared(t, response)
}

func TestRequireSession_PurgedSessionClearsCookies(t *testing.T) {
	service, _ := newTestService(t)
	handler := httpx.NewRouter(nil)
	mountAuthForTest(handler, ControllerParams{Service: service, Limiter: newTestLimiter(startRedis(t), 10)})
	sessionCookie := cookie(login(t, handler, ownerEmail, ownerPassword), SessionCookie)
	_, err := service.db.Exec(t.Context(), `DELETE FROM sessions WHERE token_hash=$1`, hashSessionToken(sessionCookie.Value))
	require.NoError(t, err)

	response := getMe(handler, sessionCookie)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, "UNAUTHENTICATED", errCode(t, response))
	requireCookiesCleared(t, response)
}

func TestRequireSession_MissingCookieSetsNoCookies(t *testing.T) {
	handler := httpx.NewRouter(nil)
	handler.GET("/probe", RequireSession(nil, false))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Empty(t, response.Result().Cookies())
}
