package model

import (
	"time"

	"github.com/google/uuid"
)

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type StepKind string

const (
	StepKindRouter        StepKind = "router"
	StepKindModel         StepKind = "model"
	StepKindTool          StepKind = "tool"
	StepKindRewrite       StepKind = "rewrite"
	StepKindCitationCheck StepKind = "citation_check"
	StepKindCompaction    StepKind = "compaction"
	StepKindTitle         StepKind = "title"
)

type StepOutcome string

const (
	StepOutcomeSuccess StepOutcome = "success"
	StepOutcomeError   StepOutcome = "error"
	StepOutcomeSkipped StepOutcome = "skipped"
)

type Run struct {
	ID                 uuid.UUID
	ConversationID     uuid.UUID
	AssistantMessageID string
	Status             RunStatus
	Route              string
	ModelRef           string
	ErrorCode          string
	InputTokens        int64
	OutputTokens       int64
	DeadlineAt         time.Time
	StartedAt          time.Time
	FinishedAt         *time.Time
	// RequestID is the X-Request-Id of the chat request that began the run (W9).
	RequestID string
	// QueuedFor is how long the run waited for a worker, measured by the database clock; only ClaimRun fills it.
	QueuedFor time.Duration
}

// Lease identifies one claim attempt. Token is unique per attempt, so a stale executor's writes are fenced out.
type Lease struct {
	RunID, Token uuid.UUID
	Owner        string
}

// ClaimResult is what a worker learns when it tries to claim a run (amendment W5).
type ClaimResult string

const (
	ClaimResultClaimed            ClaimResult = "claimed"
	ClaimResultHeldElsewhere      ClaimResult = "held_elsewhere"
	ClaimResultExpired            ClaimResult = "expired"
	ClaimResultNotRunning         ClaimResult = "not_running"
	ClaimResultQueuedPastDeadline ClaimResult = "queued_past_deadline"
)

// StreamChatRuns is the outbox topic and Redis Stream that carry chat runs to the worker.
const StreamChatRuns = "chat.runs"

// ChatRunJob is the payload of a chat.runs job. Tenant and request id travel in the outbox columns.
type ChatRunJob struct {
	RunID          uuid.UUID `json:"runId"`
	ConversationID uuid.UUID `json:"conversationId"`
	UserID         uuid.UUID `json:"userId"`
}

// BeginRunInput starts a run in one transaction. Tenant, user and request id come from ctx, never from here.
type BeginRunInput struct {
	ConversationID, AgentID uuid.UUID
	Trigger                 string
	// UserMessage is set for submit-message; RegenerateMessageID for regenerate-message.
	UserMessage         *Message
	RegenerateMessageID string
	AssistantMessageID  string
	Deadline            time.Time
}

// Triggers the chat endpoint accepts (wire contract).
const (
	TriggerSubmitMessage     = "submit-message"
	TriggerRegenerateMessage = "regenerate-message"
)

// RunSummary is the slice of the latest run that a reopened conversation shows (W7).
type RunSummary struct {
	ID        uuid.UUID
	Status    RunStatus
	ErrorCode string
	RequestID string
}

type RunStep struct {
	RunID        uuid.UUID
	Step         int
	Kind         StepKind
	Name         string
	Outcome      StepOutcome
	ErrorCode    string
	InputTokens  int64
	OutputTokens int64
	DurationMS   int64
	StartedAt    time.Time
}
