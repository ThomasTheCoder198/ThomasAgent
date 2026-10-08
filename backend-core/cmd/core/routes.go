package main

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type middlewares struct {
	sessions       *auth.Service
	cookieSecure   bool
	serviceToken   string
	health         map[string]httpx.ReadinessDependency
	requestTimeout time.Duration
}

func registerRoutes(engine *gin.Engine, c controllers, mw middlewares) {
	httpx.MountHealth(engine, mw.health)
	api := engine.Group("", httpx.LimitRequestDuration(mw.requestTimeout))
	api.POST("/api/v1/auth/login", c.auth.HandleLogin)

	session := api.Group("", auth.RequireSession(mw.sessions, mw.cookieSecure))
	session.POST("/api/v1/auth/logout", c.auth.HandleLogout)
	session.GET("/api/v1/auth/me", c.auth.HandleMe)
	session.GET("/api/v1/providers", c.providers.HandleListProviders)
	session.POST("/api/v1/providers", c.providers.HandleCreateProvider)
	session.PATCH("/api/v1/providers/:providerId", c.providers.HandleUpdateProvider)
	session.DELETE("/api/v1/providers/:providerId", c.providers.HandleDeleteProvider)
	session.POST("/api/v1/providers/:providerId/test", c.providers.HandleTestProvider)
	session.POST("/api/v1/providers/:providerId/sync", c.providers.HandleSyncProvider)
	session.GET("/api/v1/models", c.models.HandleListModels)
	session.POST("/api/v1/models", c.models.HandleCreateModel)
	session.DELETE("/api/v1/models/:modelId", c.models.HandleDeleteModel)
	session.GET("/api/v1/model-roles", c.roles.HandleListRoles)
	session.PUT("/api/v1/model-roles/:role", c.roles.HandleAssignRole)

	internal := api.Group("/internal", auth.RequireServiceToken(mw.serviceToken))
	internal.GET("/models/resolve", c.resolve.HandleResolve)
}
