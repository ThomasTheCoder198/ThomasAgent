package main

import (
	"log/slog"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"

	authhandler "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/handler"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	registryhandler "github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/handler"
	registryservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry/service"
)

type routerDeps struct {
	log                *slog.Logger
	auth               authservice.AuthService
	limiter            *authlimiter.Limiter
	cookieSecure       bool
	trustedProxyHeader string
	trustedProxyCIDRs  []netip.Prefix
	registry           registryRouterService
	serviceToken       string
	health             map[string]httpx.DependencyPinger
	metrics            httpx.HTTPMetrics

	requestTimeout time.Duration // cfg.HTTP.RequestTimeout
	maxBodyBytes   int64         // cfg.HTTP.MaxBodyBytes
}

type registryRouterService interface {
	registryservice.RegistryService
	registryservice.Resolver
}

func newRouter(d routerDeps) chi.Router {
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(d.log), httpx.LogAccess(d.log), httpx.RecordMetrics(d.metrics))
	httpx.MountHealth(r, d.health)
	// Ordinary routes carry a request deadline. Stream routes (M2) are mounted on r directly, outside this group,
	// because their lifetime is the run deadline, not an HTTP middleware.
	r.Group(func(api chi.Router) {
		api.Use(httpx.LimitRequestDuration(d.requestTimeout))
		authhandler.NewAuthHandlerWithTrustedProxies(d.auth, d.limiter, d.cookieSecure, d.trustedProxyHeader, d.trustedProxyCIDRs, d.log).Mount(api)
		api.Group(func(pr chi.Router) {
			pr.Use(authmiddleware.RequireSession(d.auth))
			registryhandler.NewRegistryHandler(d.registry, d.maxBodyBytes).Mount(pr)
		})
		registryhandler.MountInternal(api, d.registry, d.serviceToken)
	})
	return r
}
