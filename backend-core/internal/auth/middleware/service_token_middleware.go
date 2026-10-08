package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// RequireServiceToken authenticates internal callers by the shared bearer token and runs
// them in the platform tenant; browser sessions never reach these routes.
func RequireServiceToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
		parts := strings.Fields(c.Request.Header.Get(httpx.HeaderAuthorization))
		if token == "" || len(parts) != 2 || !strings.EqualFold(parts[0], httpx.AuthSchemeBearer) {
			httpx.Fail(c, errors.ErrInternalTokenInvalid)
			return
		}
		expected := sha256.Sum256([]byte(token))
		supplied := sha256.Sum256([]byte(parts[1]))
		if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
			httpx.Fail(c, errors.ErrInternalTokenInvalid)
			return
		}
		c.Request = c.Request.WithContext(tenant.WithID(c.Request.Context(), tenant.PlatformID))
		c.Next()
	}
}
