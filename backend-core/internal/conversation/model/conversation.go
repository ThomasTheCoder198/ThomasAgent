package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"

	FieldTitle  = "title"
	FieldCursor = "cursor"
	FieldLimit  = "limit"
	// TitleMaxRunes bounds a title set by rename; the runtime's own titles use CORE_CHAT_TITLE_MAX_RUNES (M2b).
	TitleMaxRunes = 200
	// LineKB is the tool-source line of the ragx KB tools (frontend components/lines.ts).
	LineKB = "kb"
)

type Conversation struct {
	ID                uuid.UUID
	AgentID           uuid.UUID
	UserID            uuid.UUID
	Title             string
	Summary           string
	SummaryThroughSeq int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Message is one AI SDK UIMessage. Parts and Metadata are stored and returned verbatim (R6).
type Message struct {
	ID        string
	Role      string
	Seq       int64
	Parts     json.RawMessage
	Metadata  json.RawMessage
	CreatedAt time.Time
}

// History is what a run needs to rebuild the model input: the conversation row and its latest messages, oldest first.
type History struct {
	Conversation Conversation
	Messages     []Message
	// PeakInputTokens is the largest input of a model step in the latest finished run (0 when none); the M2b
	// compaction trigger (B2-5) compares it with the context window (index amendment 10).
	PeakInputTokens int64
}

type ConversationSummary struct {
	ID        uuid.UUID
	Title     string
	AgentID   uuid.UUID
	Lines     []string
	UpdatedAt time.Time
}

type ScopeAgent struct {
	ID   uuid.UUID
	Name string
}

type ScopeKB struct {
	ID      uuid.UUID
	Name    string
	Missing bool
}

type ToolSource struct {
	Line string
	Name string
}

// Scope is the index's ChatScope.
type Scope struct {
	Agent       ScopeAgent
	KB          ScopeKB
	ToolSources []ToolSource
}

type ConversationDetail struct {
	ID       uuid.UUID
	Title    string
	AgentID  uuid.UUID
	Scope    Scope
	Messages []Message
	// LastRun is the conversation's latest run, nil when it has none (W7).
	LastRun *RunSummary
}

// Page asks for one page of a list; an empty Cursor is the first page.
type Page struct {
	Cursor string
	Limit  int
}

// PageMeta is meta.page of a list response; a nil Cursor marks the last page.
type PageMeta struct {
	Cursor *string
	Limit  int
}
