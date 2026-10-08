package httpx

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Stream routes are mounted outside this middleware so their lifetime follows the run deadline.
func LimitRequestDuration(limit time.Duration) Middleware {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), limit)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
