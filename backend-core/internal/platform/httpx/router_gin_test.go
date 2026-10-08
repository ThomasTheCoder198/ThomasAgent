package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ginContextKey struct{}

func TestRouter_MiddlewareContextReachesHandler(t *testing.T) {
	r := NewRouter(nil, func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ginContextKey{}, "trusted"))
		c.Next()
	})
	r.GET("/items/:id", func(c *gin.Context) {
		require.Equal(t, "trusted", c.Request.Context().Value(ginContextKey{}))
		require.Equal(t, "/items/{id}", RoutePattern(c))
		Success(c, http.StatusOK, nil)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items/123", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))
}
