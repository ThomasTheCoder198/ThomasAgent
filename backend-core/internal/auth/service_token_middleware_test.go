package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const serviceToken = "svc-token"

func sendWithAuthorization(handler http.Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/probe", nil)
	request.Header.Set("Authorization", authorization)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestServiceTokenSetsExplicitPlatformTenant(t *testing.T) {
	handler := httpx.NewRouter(nil)
	handler.GET("/internal/probe", RequireServiceToken(serviceToken), gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		got, err := tenant.ID(r.Context())
		require.NoError(t, err)
		require.Equal(t, "default", got)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/internal/probe", nil)
	request.Header.Set("Authorization", "Bearer "+serviceToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request.WithContext(tenant.WithID(request.Context(), "untrusted")))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestServiceToken_RejectsMissingBearerPrefixAndEmptyConfiguration(t *testing.T) {
	for _, token := range []string{serviceToken, ""} {
		for _, header := range []string{"", serviceToken, "Basic " + serviceToken, "Bearer ", "Bearer wrong"} {
			called := false
			router := httpx.NewRouter(func(context.Context, *errors.AppError) {})
			router.GET("/probe", RequireServiceToken(token), gin.WrapF(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
			res := sendWithAuthorization(router, header)
			require.Equal(t, http.StatusUnauthorized, res.Code, token+"/"+header)
			require.False(t, called)
			require.NotContains(t, res.Body.String(), serviceToken)
		}
	}
}

func TestServiceTokenAcceptsMixedCaseBearerAndNeverCaches(t *testing.T) {
	router := httpx.NewRouter(nil)
	router.GET("/probe", RequireServiceToken(serviceToken), gin.WrapF(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, fixture := range []struct {
		header string
		status int
	}{{"bEaReR " + serviceToken, 204}, {"Bearer wrong", 401}} {
		response := sendWithAuthorization(router, fixture.header)
		require.Equal(t, fixture.status, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
}
