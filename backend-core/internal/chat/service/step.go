package service

import (
	"context"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
)

type stepResult struct {
	response *ai.ModelResponse
	reasoned bool
}

// modelStep streams one model call: reasoning and text become events as they arrive; tool requests come back
// unexecuted (R4, contract test 2b-1).
func (r *run) modelStep(messages []*ai.Message, tools []ai.ToolRef) (stepResult, error) {
	opts := append(r.chat.Options(r.svc.deps.Config.MaxOutputTokens),
		ai.WithMessages(append([]*ai.Message{ai.NewSystemTextMessage(r.systemPrompt())}, messages...)...),
		ai.WithReturnToolRequests(true))
	if len(tools) > 0 {
		opts = append(opts, ai.WithTools(tools...))
	}
	started := r.svc.deps.Now()
	stream := &answerStream{r: r}
	var response *ai.ModelResponse
	var streamErr error
	for value, err := range genkit.GenerateStream(r.ctx, r.chat.G, opts...) {
		if err != nil {
			streamErr = err
			break
		}
		if value.Done {
			response = value.Response
			break
		}
		for _, part := range value.Chunk.Content {
			stream.part(part)
		}
	}
	stream.end()
	usage := genkitx.UsageOf(response)
	err := r.stepError(streamErr, response)
	r.logLLM(llmmodel.RoleChatDefault, r.resolved, usage, started, finishReason(response), err)
	r.addUsage(usage)
	outcome := outcomeSuccess
	if err != nil {
		outcome = outcomeError
	}
	r.recordStep(stepKindModel, r.resolved.ModelRef, outcome, err, usage, started)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{response: response, reasoned: stream.reasoned}, nil
}

// answerStream turns the chunks of one model call into events and keeps the reasoning and text parts it has open.
type answerStream struct {
	r                *run
	reasoningID      string
	reasoningStarted time.Time
	textID           string
	reasoned         bool
}

func (s *answerStream) part(part *ai.Part) {
	switch {
	case part.IsReasoning() && part.Text != "":
		s.reasoning(part.Text)
	case part.IsText() && part.Text != "":
		s.text(part.Text)
	}
}

func (s *answerStream) reasoning(delta string) {
	if s.reasoningID == "" {
		s.reasoningID, s.reasoningStarted, s.reasoned = s.r.nextID(idPrefixReason), s.r.svc.deps.Now(), true
		s.r.emit(model.ReasoningStart{ID: s.reasoningID})
	}
	s.r.emit(model.ReasoningDelta{ID: s.reasoningID, Delta: delta})
}

func (s *answerStream) text(delta string) {
	s.closeReasoning()
	if s.textID == "" {
		s.textID = s.r.nextID(idPrefixText)
		s.r.markFirstToken()
		s.r.emit(model.TextStart{ID: s.textID})
	}
	s.r.emit(model.TextDelta{ID: s.textID, Delta: delta})
}

func (s *answerStream) closeReasoning() {
	if s.reasoningID == "" {
		return
	}
	s.r.emit(model.ReasoningEnd{ID: s.reasoningID, DurationMS: s.r.svc.deps.Now().Sub(s.reasoningStarted).Milliseconds()})
	s.reasoningID = ""
}

// end closes whatever the call left open, so a stopped or failed call still leaves complete parts.
func (s *answerStream) end() {
	s.closeReasoning()
	if s.textID != "" {
		s.r.emit(model.TextEnd{ID: s.textID})
	}
}

// stepError maps what a model call ended with to the catalog. A run context that ended (Stop, deadline, cap, lease)
// is reported as the cancellation it is: the caller decides from the cause what it means.
func (r *run) stepError(streamErr error, response *ai.ModelResponse) error {
	if r.ctx.Err() != nil {
		return context.Canceled
	}
	err := genkitx.ToAppError(streamErr)
	if err == nil && response == nil { // the stream ended without a final response: a broken provider
		err = errors.ErrProviderUnavailable
	}
	return err
}

func (r *run) markFirstToken() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ttft == 0 {
		r.ttft = r.svc.deps.Now().Sub(r.started)
	}
}

func finishReason(resp *ai.ModelResponse) string {
	if resp == nil {
		return ""
	}
	return string(resp.FinishReason)
}

// logLLM writes llm.call (content stays off unless CORE_LLM_LOG_CONTENT; calllog applies the switch). It runs on the
// run context's values, so the call carries the run's request id.
func (r *run) logLLM(role llmmodel.Role, m llmmodel.ResolvedModel, usage genkitx.Usage, started time.Time, finish string, err error) {
	var cost float64
	if usage.CostUSD != nil {
		cost = *usage.CostUSD
	}
	call := calllog.LLMCall{
		RunID: r.in.Run.ID.String(), Step: r.currentStep() + 1, Provider: string(m.ProviderKind), Model: m.ModelRef,
		Role: string(role), InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CachedTokens: usage.CachedTokens, ReasoningTokens: usage.ReasoningTokens, CostUSD: cost,
		Duration: r.svc.deps.Now().Sub(started), Attempt: 1, FinishReason: finish,
	}
	if err != nil {
		call.ErrorCode = stepErrorCode(err)
	}
	r.svc.deps.Calls.LLM(context.WithoutCancel(r.ctx), call)
}
