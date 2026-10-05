package main

import (
	"log/slog"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/registry"
)

type routerDeps struct {
	log                *slog.Logger
	serviceName        string
	auth               *auth.Service
	limiter            *auth.Limiter
	cookieSecure       bool
	trustedProxyHeader string
	trustedProxyCIDRs  []netip.Prefix
	registry           *registry.Service
	serviceToken       string
	health             map[string]httpx.DependencyPinger

	requestTimeout time.Duration // cfg.HTTP.RequestTimeout
	maxBodyBytes   int64         // cfg.HTTP.MaxBodyBytes
}

func newRouter(d routerDeps) chi.Router {
	r := httpx.NewRouter(httpx.NewSlogErrorLogger(d.log), httpx.TraceRequests(d.serviceName), httpx.LogAccess(d.log))
	httpx.MountHealth(r, d.health)
	// Ordinary routes carry a request deadline. Stream routes (M2) are mounted on r directly, outside this group,
	// because their lifetime is the run deadline, not an HTTP middleware.
	r.Group(func(api chi.Router) {
		api.Use(httpx.LimitRequestDuration(d.requestTimeout))
		auth.NewHandlerWithTrustedProxies(d.auth, d.limiter, d.cookieSecure, d.trustedProxyHeader, d.trustedProxyCIDRs, d.log).Mount(api)
		api.Group(func(pr chi.Router) {
			pr.Use(auth.RequireSession(d.auth))
			registry.NewHandler(d.registry, d.maxBodyBytes).Mount(pr)
		})
		registry.MountInternal(api, d.registry, d.serviceToken)
	})
	return r
}
