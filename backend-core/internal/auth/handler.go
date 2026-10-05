package auth

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

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
	svc                *Service
	lim                *Limiter
	cookieSecure       bool
	trustedProxyHeader string
	log                *slog.Logger
	trustedProxyCIDRs  []netip.Prefix
}

func NewHandler(svc *Service, lim *Limiter, cookieSecure bool) *Handler {
	return &Handler{svc: svc, lim: lim, cookieSecure: cookieSecure}
}

func NewHandlerWithProxyHeader(svc *Service, lim *Limiter, cookieSecure bool, header string, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, lim: lim, cookieSecure: cookieSecure, trustedProxyHeader: header, log: logger}
}

func NewHandlerWithTrustedProxies(svc *Service, lim *Limiter, cookieSecure bool, header string, networks []netip.Prefix, logger *slog.Logger) *Handler {
	h := NewHandlerWithProxyHeader(svc, lim, cookieSecure, header, logger)
	h.trustedProxyCIDRs = append([]netip.Prefix(nil), networks...)
	return h
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type loginResponse struct {
	User      userDTO `json:"user"`
	CSRFToken string  `json:"csrfToken"`
	ExpiresAt string  `json:"expiresAt"`
}

func toDTO(u User) userDTO {
	return userDTO{ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName}
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/v1/auth/login", h.login)
	r.Group(func(pr chi.Router) {
		pr.Use(RequireSession(h.svc))
		pr.Post("/api/v1/auth/logout", h.logout)
		pr.Get("/api/v1/auth/me", h.me)
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(tenant.WithID(r.Context(), tenant.PlatformID))
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	ip := h.clientIP(r)
	var req loginRequest
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
			h.log.WarnContext(r.Context(), "auth login failed", slog.String("event", actionLoginFailed), slog.String("identity", h.lim.identity(req.Email)), slog.String("trace_id", trace.SpanContextFromContext(r.Context()).TraceID().String()))
		}
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.lim.ResetLogin(r.Context(), req.Email, ip); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.setCookies(w, issued)
	httpx.WriteSuccess(w, r, http.StatusOK, loginResponse{User: toDTO(user), CSRFToken: issued.CSRFToken, ExpiresAt: issued.ExpiresAt.Format(time.RFC3339)})
}

func (h *Handler) writeLimiterError(w http.ResponseWriter, r *http.Request, decision LimitDecision, err error) {
	if decision.EmailTripped {
		if auditErr := h.svc.recordFailedLogin(r.Context()); auditErr != nil && h.log != nil {
			h.log.WarnContext(r.Context(), "auth limiter audit unavailable", slog.String("trace_id", trace.SpanContextFromContext(r.Context()).TraceID().String()))
		}
	}
	appErr := errors.ToAppError(err)
	if appErr.Code == errors.CodeRateLimited {
		if seconds, ok := appErr.Details[retryAfterSeconds].(int64); ok {
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
	c, _ := r.Cookie(CookieSession)
	if err := h.svc.Logout(r.Context(), c.Value); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.clearCookies(w)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]bool{"loggedOut": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	user, _ := UserFrom(r.Context())
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]userDTO{"user": toDTO(user)})
}

func (h *Handler) setCookies(w http.ResponseWriter, s IssuedSession) {
	http.SetCookie(w, &http.Cookie{Name: CookieSession, Value: s.Token, Path: "/", Expires: s.ExpiresAt, HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: CookieCSRF, Value: s.CSRFToken, Path: "/", Expires: s.ExpiresAt, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{CookieSession, CookieCSRF} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == CookieSession, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	}
}
