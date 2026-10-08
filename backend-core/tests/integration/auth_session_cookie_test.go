package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	authutil "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/util"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

func requireCookiesCleared(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for _, name := range []string{authutil.SessionCookie, authutil.CSRFCookie} {
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
	service, _ := newAuthFixture(t)
	handler := httpx.NewRouter(nil)
	mountAuthForTest(handler, authcontroller.AuthControllerParams{AuthService: service, Limiter: newTestLimiter(startRedis(t), 10)})
	sessionCookie := cookie(login(t, handler, ownerEmail, ownerPassword), authutil.SessionCookie)
	_, err := service.db.Exec(t.Context(), `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, authmodel.HashSessionToken(sessionCookie.Value))
	require.NoError(t, err)

	response := getMe(handler, sessionCookie)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, "AUTH_SESSION_EXPIRED", errCode(t, response))
	requireCookiesCleared(t, response)
}

func TestRequireSession_PurgedSessionClearsCookies(t *testing.T) {
	service, _ := newAuthFixture(t)
	handler := httpx.NewRouter(nil)
	mountAuthForTest(handler, authcontroller.AuthControllerParams{AuthService: service, Limiter: newTestLimiter(startRedis(t), 10)})
	sessionCookie := cookie(login(t, handler, ownerEmail, ownerPassword), authutil.SessionCookie)
	_, err := service.db.Exec(t.Context(), `DELETE FROM sessions WHERE token_hash=$1`, authmodel.HashSessionToken(sessionCookie.Value))
	require.NoError(t, err)

	response := getMe(handler, sessionCookie)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, "UNAUTHENTICATED", errCode(t, response))
	requireCookiesCleared(t, response)
}

func TestRequireSession_MissingCookieSetsNoCookies(t *testing.T) {
	handler := httpx.NewRouter(nil)
	handler.GET("/probe", authmiddleware.RequireSession(nil, false))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Empty(t, response.Result().Cookies())
}
