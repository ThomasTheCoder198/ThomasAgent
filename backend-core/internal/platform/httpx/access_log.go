package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
)

const (
	accessLogMessage         = "http request"
	unmatchedRoute           = "unmatched"
	headerIdempotencyKey     = "Idempotency-Key"
	fieldMethod              = "method"
	fieldRoute               = "route"
	fieldStatus              = "status"
	fieldIdempotencyKey      = "idempotency_key"
	fieldIdempotencyRejected = "idempotency_key_rejected"
	fieldRequestIDSource     = "request_id_source"
	fieldClientIDRejected    = "client_request_id_rejected"
	requestIDSourceGenerated = "generated"
)

var validIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var ginRouteParam = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_]*)`)

type requestIDOrigin int

const (
	requestIDFromClient requestIDOrigin = iota
	requestIDGenerated
	requestIDReplaced
)

type requestIDOriginKey struct{}

func withRequestIDOrigin(ctx context.Context, origin requestIDOrigin) context.Context {
	return context.WithValue(ctx, requestIDOriginKey{}, origin)
}

func requestIDOriginFrom(ctx context.Context) requestIDOrigin {
	origin, _ := ctx.Value(requestIDOriginKey{}).(requestIDOrigin)
	return origin
}

func LogAccess(l *slog.Logger) Middleware {
	return func(c *gin.Context) {
		start := time.Now()
		defer func() {
			status := c.Writer.Status()
			l.LogAttrs(c.Request.Context(), accessLevel(status), accessLogMessage, accessAttrs(c, status, time.Since(start))...)
		}()
		c.Next()
	}
}

func accessLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func accessAttrs(c *gin.Context, status int, elapsed time.Duration) []slog.Attr {
	r := c.Request
	attrs := []slog.Attr{
		slog.String(fieldMethod, r.Method),
		slog.String(fieldRoute, RoutePattern(c)),
		slog.Int(fieldStatus, status),
		slog.Int64(logging.FieldDurationMS, elapsed.Milliseconds()),
	}
	switch requestIDOriginFrom(r.Context()) {
	case requestIDGenerated:
		attrs = append(attrs, slog.String(fieldRequestIDSource, requestIDSourceGenerated))
	case requestIDReplaced:
		attrs = append(attrs, slog.String(fieldRequestIDSource, requestIDSourceGenerated), slog.Bool(fieldClientIDRejected, true))
	}
	if key := r.Header.Get(headerIdempotencyKey); key != "" {
		if validIdempotencyKey.MatchString(key) {
			attrs = append(attrs, slog.String(fieldIdempotencyKey, key))
		} else {
			attrs = append(attrs, slog.Bool(fieldIdempotencyRejected, true))
		}
	}
	return attrs
}

// RoutePattern preserves the contract's brace syntax for route labels.
func RoutePattern(c *gin.Context) string {
	pattern := c.FullPath()
	if pattern == "" {
		return unmatchedRoute
	}
	return ginRouteParam.ReplaceAllString(pattern, "{$1}")
}
