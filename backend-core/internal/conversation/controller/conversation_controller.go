package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/dto"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const (
	PathParamConversationID = "conversationId"
	queryCursor             = "cursor"
	queryLimit              = "limit"
)

// ConversationController serves /api/v1/orgs/{orgId}/conversations; a malformed id is CONVERSATION_NOT_FOUND.
type ConversationController struct {
	conversationService service.ConversationService
	paging              config.ConversationConfig
	maxBodyBytes        int64
}

func NewConversationController(conversationService service.ConversationService, paging config.ConversationConfig, maxBodyBytes int64) *ConversationController {
	return &ConversationController{conversationService: conversationService, paging: paging, maxBodyBytes: maxBodyBytes}
}

func conversationID(ctx *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(ctx.Param(PathParamConversationID))
	if err != nil {
		return uuid.Nil, errors.ErrConversationNotFound
	}
	return id, nil
}

func (c *ConversationController) HandleListConversations(ctx *gin.Context) {
	limit, err := service.ParseLimit(ctx.Query(queryLimit), c.paging)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	list, meta, err := c.conversationService.List(ctx.Request.Context(), model.Page{Cursor: ctx.Query(queryCursor), Limit: limit})
	httpx.RespondPage(ctx, http.StatusOK, dto.NewConversationSummaryResponses(list), dto.NewPageInfo(meta), err)
}

func (c *ConversationController) HandleGetConversation(ctx *gin.Context) {
	id, err := conversationID(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	detail, err := c.conversationService.Get(ctx.Request.Context(), id)
	httpx.Respond(ctx, http.StatusOK, dto.NewConversationDetailResponse(detail), err)
}

func (c *ConversationController) HandleRenameConversation(ctx *gin.Context) {
	id, err := conversationID(ctx)
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	var request dto.RenameConversationRequest
	if err := httpx.DecodeJSON(ctx.Writer, ctx.Request, &request, c.maxBodyBytes); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	if err := c.conversationService.Rename(ctx.Request.Context(), id, request.Title); err != nil {
		httpx.Fail(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *ConversationController) HandleDeleteConversation(ctx *gin.Context) {
	id, err := conversationID(ctx)
	if err == nil {
		err = c.conversationService.Delete(ctx.Request.Context(), id)
	}
	if err != nil {
		httpx.Fail(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
