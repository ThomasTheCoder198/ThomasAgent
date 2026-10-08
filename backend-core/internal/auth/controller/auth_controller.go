package controller

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/util"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	maxLoginBodyBytes = 4 << 10
	headerRetryAfter  = "Retry-After"
)

type AuthControllerParams struct {
	AuthService        service.AuthService
	Limiter            *limiter.LoginLimiter
	CookieSecure       bool
	TrustedProxyHeader string
	TrustedProxyCIDRs  []netip.Prefix
	Logger             *slog.Logger
}

// AuthController serves login, logout and the current user.
type AuthController struct {
	authService        service.AuthService
	limiter            *limiter.LoginLimiter
	cookieSecure       bool
	trustedProxyHeader string
	trustedProxyCIDRs  []netip.Prefix
	log                *slog.Logger
}

func NewAuthController(params AuthControllerParams) *AuthController {
	return &AuthController{
		authService:        params.AuthService,
		limiter:            params.Limiter,
		cookieSecure:       params.CookieSecure,
		trustedProxyHeader: params.TrustedProxyHeader,
		trustedProxyCIDRs:  append([]netip.Prefix(nil), params.TrustedProxyCIDRs...),
		log:                params.Logger,
	}
}

func (c *AuthController) HandleLogin(ctx *gin.Context) {
	ctx.Request = ctx.Request.WithContext(tenant.WithID(ctx.Request.Context(), tenant.PlatformID))
	res, req := ctx.Writer, ctx.Request
	res.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	ip := c.clientIP(req)
	var request dto.LoginRequest
	if err := httpx.DecodeJSON(res, req, &request, maxLoginBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	decision, err := c.limiter.Check(req.Context(), request.Email, ip)
	if err != nil {
		c.writeLimiterError(ctx, decision, err)
		return
	}
	user, issued, err := c.authService.Login(req.Context(), request.Email, request.Password)
	if err != nil {
		if errors.ToAppError(err).Code == errors.CodeAuthInvalidCredentials && c.log != nil {
			c.log.WarnContext(req.Context(), "auth login failed", slog.String("event", service.AuditActionLoginFail), slog.String("identity", c.limiter.Identity(request.Email)))
		}
		httpx.Fail(ctx, err)
		return
	}
	if err := c.limiter.ResetLogin(req.Context(), request.Email, ip); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	util.SetSessionCookies(res, issued, c.cookieSecure)
	httpx.Success(ctx, http.StatusOK, dto.NewLoginResponse(user, issued))
}

func (c *AuthController) writeLimiterError(ctx *gin.Context, decision limiter.LoginLimitDecision, err error) {
	if decision.EmailTripped {
		if auditErr := c.authService.RecordFailedLogin(ctx.Request.Context()); auditErr != nil && c.log != nil {
			c.log.WarnContext(ctx.Request.Context(), "auth limiter audit unavailable")
		}
	}
	appErr := errors.ToAppError(err)
	if appErr.Code == errors.CodeRateLimited {
		if seconds, ok := appErr.Details[limiter.RetryAfterSecondsDetail].(int64); ok {
			ctx.Writer.Header().Set(headerRetryAfter, strconv.FormatInt(seconds, 10))
		}
	}
	httpx.Fail(ctx, err)
}

// HandleLogout runs behind RequireSession, so the session cookie is known to exist.
func (c *AuthController) HandleLogout(ctx *gin.Context) {
	cookie, _ := ctx.Request.Cookie(util.SessionCookie)
	if err := c.authService.Logout(ctx.Request.Context(), cookie.Value); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	util.ClearSessionCookies(ctx.Writer, c.cookieSecure)
	httpx.Success(ctx, http.StatusOK, dto.LogoutResponse{LoggedOut: true})
}

func (c *AuthController) HandleMe(ctx *gin.Context) {
	ctx.Writer.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	user, _ := middleware.UserFromContext(ctx.Request.Context())
	httpx.Success(ctx, http.StatusOK, dto.MeResponse{User: dto.NewUserResponse(user)})
}
