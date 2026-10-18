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
