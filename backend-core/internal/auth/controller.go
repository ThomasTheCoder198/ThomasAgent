package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	maxLoginBodyBytes = 4 << 10
	headerRetryAfter  = "Retry-After"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type LoginResponse struct {
	User      UserResponse `json:"user"`
	CSRFToken string       `json:"csrfToken"`
	ExpiresAt string       `json:"expiresAt"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}

type LogoutResponse struct {
	LoggedOut bool `json:"loggedOut"`
}

func newUserResponse(user User) UserResponse {
	return UserResponse{ID: user.ID.String(), Email: user.Email, DisplayName: user.DisplayName}
}

func newLoginResponse(user User, session IssuedSession) LoginResponse {
	return LoginResponse{User: newUserResponse(user), CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt.Format(time.RFC3339)}
}

type loginOperations interface {
	Login(ctx context.Context, email, password string) (User, IssuedSession, error)
	Logout(ctx context.Context, sessionToken string) error
	RecordFailedLogin(ctx context.Context) error
}

type ControllerParams struct {
	Service            loginOperations
	Limiter            *LoginLimiter
	CookieSecure       bool
	TrustedProxyHeader string
	TrustedProxyCIDRs  []netip.Prefix
	Logger             *slog.Logger
}

// Controller serves login, logout and the current user.
type Controller struct {
	service            loginOperations
	limiter            *LoginLimiter
	cookieSecure       bool
	trustedProxyHeader string
	trustedProxyCIDRs  []netip.Prefix
	log                *slog.Logger
}

func NewController(params ControllerParams) *Controller {
	return &Controller{
		service:            params.Service,
		limiter:            params.Limiter,
		cookieSecure:       params.CookieSecure,
		trustedProxyHeader: params.TrustedProxyHeader,
		trustedProxyCIDRs:  append([]netip.Prefix(nil), params.TrustedProxyCIDRs...),
		log:                params.Logger,
	}
}

func (h *Controller) HandleLogin(c *gin.Context) {
	c.Request = c.Request.WithContext(tenant.WithID(c.Request.Context(), tenant.PlatformID))
	res, req := c.Writer, c.Request
	res.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	ip := h.clientIP(req)
	var body LoginRequest
	if err := httpx.DecodeJSON(res, req, &body, maxLoginBodyBytes); err != nil {
		httpx.Fail(c, err)
		return
	}
	decision, err := h.limiter.Check(req.Context(), body.Email, ip)
	if err != nil {
		h.writeLimiterError(c, decision, err)
		return
	}
	user, issued, err := h.service.Login(req.Context(), body.Email, body.Password)
	if err != nil {
		if errors.ToAppError(err).Code == errors.CodeAuthInvalidCredentials && h.log != nil {
			h.log.WarnContext(req.Context(), "auth login failed", slog.String("event", auditActionLoginFail), slog.String("identity", h.limiter.Identity(body.Email)))
		}
		httpx.Fail(c, err)
		return
	}
	if err := h.limiter.ResetLogin(req.Context(), body.Email, ip); err != nil {
		httpx.Fail(c, err)
		return
	}
	setSessionCookies(res, issued, h.cookieSecure)
	httpx.Success(c, http.StatusOK, newLoginResponse(user, issued))
}

func (h *Controller) writeLimiterError(c *gin.Context, decision LoginLimitDecision, err error) {
	if decision.EmailTripped {
		if auditErr := h.service.RecordFailedLogin(c.Request.Context()); auditErr != nil && h.log != nil {
			h.log.WarnContext(c.Request.Context(), "auth limiter audit unavailable")
		}
	}
	appErr := errors.ToAppError(err)
	if appErr.Code == errors.CodeRateLimited {
		if seconds, ok := appErr.Details[retryAfterSecondsDetail].(int64); ok {
			c.Writer.Header().Set(headerRetryAfter, strconv.FormatInt(seconds, 10))
		}
	}
	httpx.Fail(c, err)
}

// HandleLogout runs behind RequireSession, so the session cookie is known to exist.
func (h *Controller) HandleLogout(c *gin.Context) {
	cookie, _ := c.Request.Cookie(SessionCookie)
	if err := h.service.Logout(c.Request.Context(), cookie.Value); err != nil {
		httpx.Fail(c, err)
		return
	}
	clearSessionCookies(c.Writer, h.cookieSecure)
	httpx.Success(c, http.StatusOK, LogoutResponse{LoggedOut: true})
}

func (h *Controller) HandleMe(c *gin.Context) {
	c.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	user, _ := UserFromContext(c.Request.Context())
	httpx.Success(c, http.StatusOK, MeResponse{User: newUserResponse(user)})
}
