package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

type RenameConversationRequest struct {
	Title string `json:"title"`
}

type ConversationSummaryResponse struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	AgentID   uuid.UUID `json:"agentId"`
	Lines     []string  `json:"lines"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewConversationSummaryResponses(list []model.ConversationSummary) []ConversationSummaryResponse {
	out := make([]ConversationSummaryResponse, 0, len(list))
	for _, c := range list {
		out = append(out, ConversationSummaryResponse(c))
	}
	return out
}

func NewPageInfo(meta model.PageMeta) httpx.PageInfo { return httpx.PageInfo(meta) }

type ScopeAgentResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ScopeKBResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Missing bool      `json:"missing"`
}

type ToolSourceResponse struct {
	Line string `json:"line"`
	Name string `json:"name"`
}

// ChatScopeResponse is the index's ChatScope.
type ChatScopeResponse struct {
	Agent       ScopeAgentResponse   `json:"agent"`
	KB          ScopeKBResponse      `json:"kb"`
	ToolSources []ToolSourceResponse `json:"toolSources"`
}

func NewChatScopeResponse(scope model.Scope) ChatScopeResponse {
	sources := make([]ToolSourceResponse, 0, len(scope.ToolSources))
	for _, source := range scope.ToolSources {
		sources = append(sources, ToolSourceResponse(source))
	}
	return ChatScopeResponse{Agent: ScopeAgentResponse(scope.Agent), KB: ScopeKBResponse(scope.KB), ToolSources: sources}
}

// MessageResponse is an AI SDK UIMessage: parts and metadata are passed through as stored (R6).
type MessageResponse struct {
	ID       string          `json:"id"`
	Role     string          `json:"role"`
	Parts    json.RawMessage `json:"parts"`
	Metadata json.RawMessage `json:"metadata"`
}

type ConversationDetailResponse struct {
	ID       uuid.UUID         `json:"id"`
	Title    string            `json:"title"`
	AgentID  uuid.UUID         `json:"agentId"`
	Scope    ChatScopeResponse `json:"scope"`
	Messages []MessageResponse `json:"messages"`
}

func NewConversationDetailResponse(d model.ConversationDetail) ConversationDetailResponse {
	messages := make([]MessageResponse, 0, len(d.Messages))
	for _, m := range d.Messages {
		messages = append(messages, MessageResponse{ID: m.ID, Role: m.Role, Parts: m.Parts, Metadata: m.Metadata})
	}
	return ConversationDetailResponse{ID: d.ID, Title: d.Title, AgentID: d.AgentID, Scope: NewChatScopeResponse(d.Scope), Messages: messages}
}
