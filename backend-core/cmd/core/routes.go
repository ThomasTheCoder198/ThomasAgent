package main

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type middlewares struct {
	auth           *auth.Service
	serviceToken   string
	health         map[string]httpx.DependencyPinger
	requestTimeout time.Duration
}

func registerRoutes(engine *gin.Engine, c controllers, mw middlewares) {
	httpx.MountHealth(engine, mw.health)
	api := engine.Group("", httpx.LimitRequestDuration(mw.requestTimeout))
	api.POST("/api/v1/auth/login", c.auth.HandleLogin)
	session := api.Group("", auth.RequireSession(mw.auth))
	session.POST("/api/v1/auth/logout", c.auth.HandleLogout)
	session.GET("/api/v1/auth/me", c.auth.HandleMe)
	session.GET("/api/v1/providers", c.provider.HandleListProviders)
	session.POST("/api/v1/providers", c.provider.HandleCreateProvider)
	session.PATCH("/api/v1/providers/:providerId", c.provider.HandleUpdateProvider)
	session.DELETE("/api/v1/providers/:providerId", c.provider.HandleDeleteProvider)
	session.POST("/api/v1/providers/:providerId/test", c.provider.HandleTestProvider)
	session.POST("/api/v1/providers/:providerId/sync", c.provider.HandleSyncProvider)
	session.GET("/api/v1/models", c.model.HandleListModels)
	session.POST("/api/v1/models", c.model.HandleCreateModel)
	session.DELETE("/api/v1/models/:modelId", c.model.HandleDeleteModel)
	session.GET("/api/v1/model-roles", c.modelRole.HandleListRoles)
	session.PUT("/api/v1/model-roles/:role", c.modelRole.HandleAssignRole)
	internal := api.Group("/internal", auth.RequireServiceToken(mw.serviceToken))
	internal.GET("/models/resolve", c.internalModel.HandleResolve)
}
