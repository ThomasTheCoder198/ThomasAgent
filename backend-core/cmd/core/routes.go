package main

import (
	"time"

	"github.com/gin-gonic/gin"

	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type middlewares struct {
	authService    authservice.AuthService
	cookieSecure   bool
	health         map[string]httpx.ReadinessDependency
	requestTimeout time.Duration
}

func registerRoutes(engine *gin.Engine, c ControllerContainer, mw middlewares) {
	httpx.MountHealth(engine, mw.health)
	api := engine.Group("", httpx.LimitRequestDuration(mw.requestTimeout))
	api.POST("/api/v1/auth/login", c.AuthController.HandleLogin)

	session := api.Group("", authmiddleware.RequireSession(mw.authService, mw.cookieSecure))
	session.POST("/api/v1/auth/logout", c.AuthController.HandleLogout)
	session.GET("/api/v1/auth/me", c.AuthController.HandleMe)
	session.GET("/api/v1/providers", c.ProviderController.HandleListProviders)
	session.POST("/api/v1/providers", c.ProviderController.HandleCreateProvider)
	session.PATCH("/api/v1/providers/:providerId", c.ProviderController.HandleUpdateProvider)
	session.DELETE("/api/v1/providers/:providerId", c.ProviderController.HandleDeleteProvider)
	session.POST("/api/v1/providers/:providerId/test", c.ProviderController.HandleTestProvider)
	session.POST("/api/v1/providers/:providerId/sync", c.ProviderController.HandleSyncProvider)
	session.GET("/api/v1/models", c.LLMModelController.HandleListModels)
	session.POST("/api/v1/models", c.LLMModelController.HandleCreateModel)
	session.DELETE("/api/v1/models/:modelId", c.LLMModelController.HandleDeleteModel)
	session.GET("/api/v1/model-roles", c.RoleController.HandleListRoles)
	session.PUT("/api/v1/model-roles/:role", c.RoleController.HandleAssignRole)
}
