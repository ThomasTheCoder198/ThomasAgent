package service

import (
	"context"
	"slices"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	systemonex "github.com/firebase/genkit/go/plugins/systemone/exp"
	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
)

const (
	probePrompt          = "Xin chào"
	probeMaxOutputTokens = 16
	probeRole            = "probe"
	reasonNotTestable    = "not_testable_in_core"
	detailReasonKey      = "reason"
)

// probeDecision is the smallest System One question: one yes/no.
type probeDecision struct {
	Greeting systemonex.Noul `json:"greeting" jsonschema_description:"Is this message a greeting?"`
}

type ModelProbeResolver interface {
	ResolveModel(ctx context.Context, modelID uuid.UUID) (model.ResolvedModel, error)
}

type ProbeModels interface {
	Chat(ctx context.Context, m model.ResolvedModel) (genkitx.ChatModel, error)
	Decision(ctx context.Context, m model.ResolvedModel) (genkitx.DecisionModel, error)
}

type LLMCallLogger interface {
	LLM(ctx context.Context, call calllog.LLMCall)
}

type ProbeResult struct {
	LatencyMS int64
}

// ModelProbeService is the Settings page's Test button: one tiny real call to a chat or decision model.
type ModelProbeService interface {
	Probe(ctx context.Context, modelID uuid.UUID) (ProbeResult, error)
}

type ModelProbeServiceImpl struct {
	resolver ModelProbeResolver
	models   ProbeModels
	calls    LLMCallLogger
	timeout  time.Duration
}

func NewModelProbeService(resolver ModelProbeResolver, models ProbeModels, calls LLMCallLogger, timeout time.Duration) ModelProbeService {
	return &ModelProbeServiceImpl{resolver: resolver, models: models, calls: calls, timeout: timeout}
}

func (s *ModelProbeServiceImpl) Probe(ctx context.Context, modelID uuid.UUID) (ProbeResult, error) {
	resolved, err := s.resolver.ResolveModel(ctx, modelID)
	if err != nil {
		return ProbeResult{}, err
	}
	var call func(context.Context, model.ResolvedModel) (genkitx.Usage, error)
	switch {
	case slices.Contains(resolved.Capabilities, model.CapabilityChat):
		call = s.probeChat
	case slices.Contains(resolved.Capabilities, model.CapabilityDecision):
		call = s.probeDecision
	default: // embedding and rerank models belong to ragx's registry
		return ProbeResult{}, errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{detailReasonKey: reasonNotTestable})
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	start := time.Now()
	usage, err := call(ctx, resolved)
	elapsed := time.Since(start)
	err = genkitx.ToAppError(err)
	s.calls.LLM(ctx, calllog.LLMCall{
		Provider: string(resolved.ProviderKind), Model: resolved.ModelRef, Role: probeRole,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, Duration: elapsed,
		Attempt: 1, ErrorCode: codeOf(err),
	})
	if err != nil {
		return ProbeResult{}, err
	}
	return ProbeResult{LatencyMS: elapsed.Milliseconds()}, nil
}

func (s *ModelProbeServiceImpl) probeChat(ctx context.Context, m model.ResolvedModel) (genkitx.Usage, error) {
	chat, err := s.models.Chat(ctx, m)
	if err != nil {
		return genkitx.Usage{}, err
	}
	opts := append(chat.Options(probeMaxOutputTokens), ai.WithMessages(ai.NewUserTextMessage(probePrompt)))
	resp, err := genkit.Generate(ctx, chat.G, opts...)
	return genkitx.UsageOf(resp), err
}

func (s *ModelProbeServiceImpl) probeDecision(ctx context.Context, m model.ResolvedModel) (genkitx.Usage, error) {
	decision, err := s.models.Decision(ctx, m)
	if err != nil {
		return genkitx.Usage{}, err
	}
	_, resp, err := genkit.GenerateData[probeDecision](ctx, decision.G, ai.WithModelName(decision.Name),
		ai.WithMessages(ai.NewUserTextMessage(probePrompt)))
	return genkitx.UsageOf(resp), err
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return string(errors.CodeOf(err))
}
