package controller

import (
	stderrors "errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/appproxy/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const fieldBody = "body"

// AppProxyController serves /api/v1/orgs/{orgId}/apps/ragx/{path}: it reads the request, calls the service and
// writes the app's answer back unchanged.
type AppProxyController struct {
	appProxyService service.AppProxyService
	maxBodyBytes    int64
}

func NewAppProxyController(appProxyService service.AppProxyService, maxBodyBytes int64) *AppProxyController {
	return &AppProxyController{appProxyService: appProxyService, maxBodyBytes: maxBodyBytes}
}

func (c *AppProxyController) HandleForward(ctx *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, c.maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if stderrors.As(err, &tooLarge) {
			httpx.Fail(ctx, errors.ErrPayloadTooLarge.WithCause(err))
			return
		}
		httpx.Fail(ctx, errors.FieldError(fieldBody, errors.FieldCodeInvalid).WithCause(err))
		return
	}
	result, err := c.appProxyService.Forward(ctx.Request.Context(), model.AppCall{
		Method: ctx.Request.Method, Path: ctx.Param(model.PathParamAppPath), RawQuery: ctx.Request.URL.RawQuery,
		Route: ctx.FullPath(), ContentType: ctx.GetHeader(httpx.HeaderContentType),
		IdempotencyKey: ctx.GetHeader(model.HeaderIdempotencyKey), AcceptLanguage: ctx.GetHeader(httpx.HeaderAcceptLanguage), Body: body,
	})
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	header := ctx.Writer.Header()
	header.Set(httpx.HeaderCacheControl, httpx.CacheControlNoStore)
	header.Set(httpx.HeaderContentTypeOptions, httpx.ContentTypeOptionsNoSniff)
	if result.ContentType != "" {
		header.Set(httpx.HeaderContentType, result.ContentType)
	}
	ctx.Status(result.Status)
	ctx.Writer.WriteHeaderNow()
	if result.Status == http.StatusNoContent || ctx.Request.Method == http.MethodHead {
		return
	}
	_, _ = ctx.Writer.Write(result.Body)
}
