package httpx

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	LivenessPath                = "/healthz"
	ReadinessPath               = "/readyz"
	LivenessStatus              = "ok"
	ReadinessStatus             = "ready"
	DependencyUnreachableStatus = "unreachable"
)

type ReadinessDependency interface {
	Ping(ctx context.Context) error
}

func MountHealth(r gin.IRoutes, deps map[string]ReadinessDependency) {
	r.GET(LivenessPath, func(c *gin.Context) {
		Success(c, http.StatusOK, map[string]string{"status": LivenessStatus})
	})
	r.GET(ReadinessPath, func(c *gin.Context) {
		failing := map[string]any{}
		for name, dep := range deps {
			if err := dep.Ping(c.Request.Context()); err != nil {
				failing[name] = DependencyUnreachableStatus
			}
		}
		if len(failing) > 0 {
			Fail(c, errors.ErrProviderUnavailable.WithDetails(failing))
			return
		}
		Success(c, http.StatusOK, map[string]string{"status": ReadinessStatus})
	})
}
