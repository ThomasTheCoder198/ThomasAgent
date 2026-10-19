package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

// RunStore is the part of conversationservice.ChatStore the executor uses (the consumer declares it, rule 2). The
// worker side of the store is fenced by the lease: RecordStep and FinishRun return errors.ErrRunLeaseLost, and write
// nothing, once another executor owns the run.
type RunStore interface {
	LoadHistory(ctx context.Context, conversationID uuid.UUID, turns int) (convmodel.History, error)
	RecordStep(ctx context.Context, lease convmodel.Lease, step convmodel.RunStep) error
	FinishRun(ctx context.Context, lease convmodel.Lease, run convmodel.Run, assistant convmodel.Message) error
}

type AgentReader interface {
	Get(ctx context.Context, id uuid.UUID) (agentmodel.Agent, error)
}

type ModelResolver interface {
	Resolve(ctx context.Context, role llmmodel.Role) (llmmodel.ResolvedModel, error)
	ResolveModel(ctx context.Context, modelID uuid.UUID) (llmmodel.ResolvedModel, error)
}

type ModelRegistry interface {
	Chat(ctx context.Context, m llmmodel.ResolvedModel) (genkitx.ChatModel, error)
}

type ToolCaller interface {
	CallTool(ctx context.Context, kbID uuid.UUID, tool ragx.ToolName, input json.RawMessage) (json.RawMessage, error)
}

type CallLogger interface {
	LLM(ctx context.Context, call calllog.LLMCall)
}
