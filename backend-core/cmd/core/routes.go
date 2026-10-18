package main

import (
	"time"

	"github.com/gin-gonic/gin"

	appmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/model"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	orgmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

// appProxyRoute is the generic app data plane (spec §14.2); M3a adds other apps beside "ragx".
const appProxyRoute = "/api/v1/orgs/:orgId/apps/ragx/*path"

type middlewares struct {
	authService    authservice.AuthService
	cookieSecure   bool
	health         map[string]httpx.ReadinessDependency
	requestTimeout time.Duration
	// organizations is kept for RequireOrg, which the app proxy route adds behind the session group.
	organizations orgmiddleware.OrganizationLookup
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
	session.PATCH("/api/v1/models/:modelId", c.LLMModelController.HandleUpdateModel)
	session.DELETE("/api/v1/models/:modelId", c.LLMModelController.HandleDeleteModel)
	session.GET("/api/v1/model-roles", c.RoleController.HandleListRoles)
	session.PUT("/api/v1/model-roles/:role", c.RoleController.HandleAssignRole)
	session.DELETE("/api/v1/model-roles/:role", c.RoleController.HandleClearRole)
	session.GET("/api/v1/orgs", c.OrganizationController.HandleListOrganizations)
	session.POST("/api/v1/orgs", c.OrganizationController.HandleCreateOrganization)
	session.GET("/api/v1/orgs/:orgId", c.OrganizationController.HandleGetOrganization)
	session.PATCH("/api/v1/orgs/:orgId", c.OrganizationController.HandleUpdateOrganization)
	session.POST("/api/v1/orgs/:orgId/provision", c.OrganizationController.HandleProvisionOrganization)

	// Session (with CSRF) first, then RequireOrg: an unknown or unready org never reaches the proxy.
	orgScoped := session.Group("", orgmiddleware.RequireOrg(mw.organizations))
	for _, method := range appmodel.ForwardedMethods() {
		orgScoped.Handle(method, appProxyRoute, c.AppProxyController.HandleForward)
	}
	orgScoped.GET("/api/v1/orgs/:orgId/model-roles", c.OrgRoleController.HandleListOrgRoles)
	orgScoped.PUT("/api/v1/orgs/:orgId/model-roles/:role", c.OrgRoleController.HandleAssignOrgRole)
	orgScoped.DELETE("/api/v1/orgs/:orgId/model-roles/:role", c.OrgRoleController.HandleClearOrgRole)
	orgScoped.GET("/api/v1/orgs/:orgId/agents", c.AgentController.HandleListAgents)
	orgScoped.POST("/api/v1/orgs/:orgId/agents", c.AgentController.HandleCreateAgent)
	orgScoped.GET("/api/v1/orgs/:orgId/agents/:agentId", c.AgentController.HandleGetAgent)
	orgScoped.PATCH("/api/v1/orgs/:orgId/agents/:agentId", c.AgentController.HandleUpdateAgent)
	orgScoped.DELETE("/api/v1/orgs/:orgId/agents/:agentId", c.AgentController.HandleDeleteAgent)
	orgScoped.GET("/api/v1/orgs/:orgId/conversations", c.ConversationController.HandleListConversations)
	orgScoped.GET("/api/v1/orgs/:orgId/conversations/:conversationId", c.ConversationController.HandleGetConversation)
	orgScoped.PATCH("/api/v1/orgs/:orgId/conversations/:conversationId", c.ConversationController.HandleRenameConversation)
	orgScoped.DELETE("/api/v1/orgs/:orgId/conversations/:conversationId", c.ConversationController.HandleDeleteConversation)
}
