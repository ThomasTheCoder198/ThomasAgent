package controller

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	maxLoginBodyBytes   = 4 << 10
	headerRetryAfter    = "Retry-After"
	invalidPeerIdentity = "invalid-peer"
)

type authOperations interface {
	Login(context.Context, string, string) (model.User, model.IssuedSession, error)
	Logout(context.Context, string) error
	RecordFailedLogin(context.Context) error
}

type AuthControllerParams struct {
	Service            authOperations
	Limiter            *limiter.Limiter
	CookieSecure       bool
	TrustedProxyHeader string
	TrustedProxyCIDRs  []netip.Prefix
	Logger             *slog.Logger
}

type AuthController struct {
	svc                authOperations
	lim                *limiter.Limiter
	cookieSecure       bool
	trustedProxyHeader string
	trustedProxyCIDRs  []netip.Prefix
	log                *slog.Logger
}

func NewAuthController(p AuthControllerParams) *AuthController {
	return &AuthController{
		svc:                p.Service,
		lim:                p.Limiter,
		cookieSecure:       p.CookieSecure,
		trustedProxyHeader: p.TrustedProxyHeader,
		trustedProxyCIDRs:  append([]netip.Prefix(nil), p.TrustedProxyCIDRs...),
		log:                p.Logger,
	}
}

func (h *AuthController) HandleLogin(c *gin.Context) {
	w, r := c.Writer, c.Request
	c.Request = r.WithContext(tenant.WithID(r.Context(), tenant.PlatformID))
	r = c.Request
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	ip := h.clientIP(r)
	var req dto.LoginRequest
	if err := httpx.DecodeJSON(w, r, &req, maxLoginBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	decision, err := h.lim.Check(r.Context(), req.Email, ip)
	if err != nil {
		h.writeLimiterError(c, decision, err)
		return
	}
	user, issued, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.ToAppError(err).Code == errors.CodeAuthInvalidCredentials && h.log != nil {
			h.log.WarnContext(r.Context(), "auth login failed", slog.String("event", model.ActionLoginFailed), slog.String("identity", h.lim.Identity(req.Email)))
		}
		httpx.Fail(c, err)
		return
	}
	if err := h.lim.ResetLogin(r.Context(), req.Email, ip); err != nil {
		httpx.Fail(c, err)
		return
	}
	h.setCookies(w, issued)
	httpx.Success(c, http.StatusOK, dto.FromIssuedSession(user, issued))
}

func (h *AuthController) writeLimiterError(c *gin.Context, decision limiter.LimitDecision, err error) {
	w, r := c.Writer, c.Request
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
	httpx.Fail(c, err)
}

func (h *AuthController) clientIP(r *http.Request) string {
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

func (h *AuthController) isTrustedProxy(ip netip.Addr) bool {
	for _, network := range h.trustedProxyCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *AuthController) HandleLogout(c *gin.Context) {
	w, r := c.Writer, c.Request
	cookie, _ := r.Cookie(middleware.CookieSession)
	if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
		httpx.Fail(c, err)
		return
	}
	h.clearCookies(w)
	httpx.Success(c, http.StatusOK, dto.LogoutResponse{LoggedOut: true})
}

func (h *AuthController) HandleMe(c *gin.Context) {
	w, r := c.Writer, c.Request
	w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	user, _ := middleware.UserFrom(r.Context())
	httpx.Success(c, http.StatusOK, dto.MeResponse{User: dto.FromUser(user)})
}

func (h *AuthController) setCookies(w http.ResponseWriter, s model.IssuedSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CookieSession,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.ExpiresAt,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CookieCSRF,
		Value:    s.CSRFToken,
		Path:     "/",
		Expires:  s.ExpiresAt,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthController) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{middleware.CookieSession, middleware.CookieCSRF} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name == middleware.CookieSession,
			Secure:   h.cookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}
