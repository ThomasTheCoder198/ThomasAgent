package httpx

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

type HTTPMetrics interface {
	ObserveHTTP(method, route string, status int, elapsed time.Duration, requestID string)
}

func RecordMetrics(rec HTTPMetrics) Middleware {
	return func(c *gin.Context) {
		if rec == nil {
			c.Next()
			return
		}
		start := time.Now()
		defer func() {
			rec.ObserveHTTP(c.Request.Method, RoutePattern(c), c.Writer.Status(), time.Since(start), logging.RequestIDFromContext(c.Request.Context()))
		}()
		c.Next()
	}
}
