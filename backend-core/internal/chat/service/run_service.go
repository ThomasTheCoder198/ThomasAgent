package service

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
)

const (
	messageIDMaxLen = 128
	roleUser        = "user"
	roleAssistant   = "assistant"
	fieldMessage    = "message"
	fieldMessageID  = "messageId"
	fieldTrigger    = "trigger"
	detailRole      = "role"
)

// RunService is the agent run. Execute is the chat.runs executor of `core worker`: it runs one claimed run (amendment
// W, ruling PW-2b4). Validate and CheckModel are the pre-run checks the chat endpoint makes before BeginRun, so a bad
// request or a model that cannot serve it is a plain JSON error instead of a failed run (index R5).
type RunService interface {
	worker.Executor
	Validate(req model.ChatRequest) error
	CheckModel(ctx context.Context, agentID uuid.UUID, requestedModelID *uuid.UUID) error
}

type RunDeps struct {
	Store       RunStore
	Agents      AgentReader
	Models      ModelResolver
	Registry    ModelRegistry
	Tools       ToolCaller
	Calls       CallLogger
	Config      config.ChatConfig
	ToolTimeout time.Duration
	Log         *slog.Logger
	Now         func() time.Time
}

type RunServiceImpl struct {
	deps  RunDeps
	tools *toolCatalog
}

func NewRunService(deps RunDeps) (RunService, error) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	catalog, err := newToolCatalog()
	if err != nil {
		return nil, err
	}
	return &RunServiceImpl{deps: deps, tools: catalog}, nil
}

// Validate checks the shape of a chat request. Failures are VALIDATION_FAILED with the offending field.
func (s *RunServiceImpl) Validate(req model.ChatRequest) error {
	switch req.Trigger {
	case model.TriggerSubmit:
		text := strings.TrimSpace(req.Message.Text)
		if text == "" {
			return errors.FieldError(fieldMessage, errors.FieldCodeRequired)
		}
		if utf8.RuneCountInString(text) > s.deps.Config.MessageMaxRunes || req.Message.ID == "" ||
			len(req.Message.ID) > messageIDMaxLen {
			return errors.FieldError(fieldMessage, errors.FieldCodeInvalid)
		}
	case model.TriggerRegenerate:
		if req.MessageID == "" || len(req.MessageID) > messageIDMaxLen {
			return errors.FieldError(fieldMessageID, errors.FieldCodeRequired)
		}
	default:
		return errors.FieldError(fieldTrigger, errors.FieldCodeInvalid)
	}
	return nil
}

// CheckModel proves the chat model of a would-be run is usable now: the Agent exists and the model resolves, can chat
// with tools and has a provider adapter. The worker repeats the resolution when the run executes.
func (s *RunServiceImpl) CheckModel(ctx context.Context, agentID uuid.UUID, requestedModelID *uuid.UUID) error {
	agent, err := s.deps.Agents.Get(ctx, agentID)
	if err != nil {
		return err
	}
	_, _, err = s.chatModel(ctx, agent, requestedModelID)
	return err
}

// chatModel follows B2-8: the request's model, then the Agent's override, then the org/platform role.
func (s *RunServiceImpl) chatModel(ctx context.Context, agent agentmodel.Agent, requested *uuid.UUID) (llmmodel.ResolvedModel, genkitx.ChatModel, error) {
	var resolved llmmodel.ResolvedModel
	var err error
	switch override, ok := agent.ModelOverrides[llmmodel.RoleChatDefault]; {
	case requested != nil:
		resolved, err = s.deps.Models.ResolveModel(ctx, *requested)
	case ok:
		resolved, err = s.deps.Models.ResolveModel(ctx, override)
	default:
		resolved, err = s.deps.Models.Resolve(ctx, llmmodel.RoleChatDefault)
	}
	if err != nil {
		return llmmodel.ResolvedModel{}, genkitx.ChatModel{}, err
	}
	if !slices.Contains(resolved.Capabilities, llmmodel.CapabilityChat) || !slices.Contains(resolved.Capabilities, llmmodel.CapabilityTools) {
		return llmmodel.ResolvedModel{}, genkitx.ChatModel{}, errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{
			detailRole: string(llmmodel.RoleChatDefault),
		})
	}
	chat, err := s.deps.Registry.Chat(ctx, resolved)
	return resolved, chat, err
}

// Execute runs one claimed run to its end and returns the terminal event the handler writes (the run's end is stored
// first, by the fenced FinishRun). A lost lease returns errors.ErrRunLeaseLost and writes nothing.
func (s *RunServiceImpl) Execute(ctx context.Context, in worker.ExecuteInput) (model.Event, error) {
	return s.newRun(ctx, in).execute()
}
