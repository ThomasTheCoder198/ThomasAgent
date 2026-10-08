package handler

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	maxLoginBodyBytes   = 4 << 10
	headerRetryAfter    = "Retry-After"
	invalidPeerIdentity = "invalid-peer"
)

type Handler struct {
	svc                service.AuthService
	lim                *limiter.Limiter
	cookieSecure       bool
	trustedProxyHeader string
	log                *slog.Logger
	trustedProxyCIDRs  []netip.Prefix
}

func NewAuthHandler(svc service.AuthService, lim *limiter.Limiter, cookieSecure bool) *Handler {
	return &Handler{svc: svc, lim: lim, cookieSecure: cookieSecure}
}

func NewAuthHandlerWithProxyHeader(svc service.AuthService, lim *limiter.Limiter, cookieSecure bool, header string, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, lim: lim, cookieSecure: cookieSecure, trustedProxyHeader: header, log: logger}
}

func NewAuthHandlerWithTrustedProxies(svc service.AuthService, lim *limiter.Limiter, cookieSecure bool, header string, networks []netip.Prefix, logger *slog.Logger) *Handler {
	h := NewAuthHandlerWithProxyHeader(svc, lim, cookieSecure, header, logger)
	h.trustedProxyCIDRs = append([]netip.Prefix(nil), networks...)
	return h
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/v1/auth/login", h.login)
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.RequireSession(h.svc))
		pr.Post("/api/v1/auth/logout", h.logout)
		pr.Get("/api/v1/auth/me", h.me)
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(tenant.WithID(r.Context(), tenant.PlatformID))
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	ip := h.clientIP(r)
	var req dto.LoginRequest
	if err := httpx.DecodeJSON(w, r, &req, maxLoginBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	decision, err := h.lim.Check(r.Context(), req.Email, ip)
	if err != nil {
		h.writeLimiterError(w, r, decision, err)
		return
	}
	user, issued, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.ToAppError(err).Code == errors.CodeAuthInvalidCredentials && h.log != nil {
			h.log.WarnContext(r.Context(), "auth login failed", slog.String("event", model.ActionLoginFailed), slog.String("identity", h.lim.Identity(req.Email)))
		}
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.lim.ResetLogin(r.Context(), req.Email, ip); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.setCookies(w, issued)
	httpx.WriteSuccess(w, r, http.StatusOK, dto.FromIssuedSession(user, issued))
}

func (h *Handler) writeLimiterError(w http.ResponseWriter, r *http.Request, decision limiter.LimitDecision, err error) {
	if decision.EmailTripped {
		if auditErr := h.svc.RecordFailedLogin(r.Context()); auditErr != nil && h.log != nil {
			h.log.WarnContext(r.Context(), "auth limiter audit unavailable")
		}
	}
	appErr := errors.ToAppError(err)
	if appErr.Code == errors.CodeRateLimited {
		if seconds, ok := appErr.Details[limiter.RetryAfterSeconds].(int64); ok {
			w.Header().Set(headerRetryAfter, strconv.FormatInt(seconds, 10))
		}
	}
	httpx.WriteError(w, r, err)
}

func (h *Handler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return invalidPeerIdentity
	}
	peer = peer.Unmap()
	if h.trustedProxyHeader == "" || !h.isTrustedProxy(peer) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(r.Header.Values(h.trustedProxyHeader), ","), ",")
	for index := len(hops) - 1; index >= 0; index-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if err != nil {
			return peer.String()
		}
		hop = hop.Unmap()
		if !h.isTrustedProxy(hop) {
			return hop.String()
		}
	}
	return peer.String()
}

func (h *Handler) isTrustedProxy(ip netip.Addr) bool {
	for _, network := range h.trustedProxyCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(middleware.CookieSession)
	if err := h.svc.Logout(r.Context(), c.Value); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.clearCookies(w)
	httpx.WriteSuccess(w, r, http.StatusOK, dto.LogoutResponse{LoggedOut: true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	user, _ := middleware.UserFrom(r.Context())
	httpx.WriteSuccess(w, r, http.StatusOK, dto.MeResponse{User: dto.FromUser(user)})
}

func (h *Handler) setCookies(w http.ResponseWriter, s model.IssuedSession) {
	http.SetCookie(w, &http.Cookie{Name: middleware.CookieSession, Value: s.Token, Path: "/", Expires: s.ExpiresAt, HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: middleware.CookieCSRF, Value: s.CSRFToken, Path: "/", Expires: s.ExpiresAt, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{middleware.CookieSession, middleware.CookieCSRF} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == middleware.CookieSession, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	}
}
