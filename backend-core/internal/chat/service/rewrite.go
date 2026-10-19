package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"unicode/utf8"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

const (
	rewritePrompt       = "Viết lại truy vấn tìm kiếm sau thành một truy vấn khác, đồng nghĩa và cụ thể hơn, cùng ngôn ngữ. Chỉ trả về truy vấn.\n\nTruy vấn: "
	rewriteMaxRunes     = 1000
	rewriteOutputTokens = 64
	queryField          = "query"
)

type searchFlag struct {
	InsufficientEvidence bool `json:"insufficientEvidence"`
}

// fastModel resolves chat.fast once per run; unassigned or unusable means nil (B2-13).
func (r *run) fastModel(ctx context.Context) *genkitx.ChatModel {
	r.fastOnce.Do(func() {
		resolved, err := r.resolveFast(ctx)
		if err != nil {
			return
		}
		chat, err := r.svc.deps.Registry.Chat(ctx, resolved)
		if err != nil {
			return
		}
		r.fast, r.fastResolved = &chat, resolved
	})
	return r.fast
}

// resolveFast honours the Agent's chat.fast pin like chatModel honours chat.default (index amendment 12). chat.fast
// is best-effort, so a pin that no longer resolves (its model was deleted, A2-10) falls back to the org/platform role.
func (r *run) resolveFast(ctx context.Context) (llmmodel.ResolvedModel, error) {
	if pinned, ok := r.agent.ModelOverrides[llmmodel.RoleChatFast]; ok {
		if resolved, err := r.svc.deps.Models.ResolveModel(ctx, pinned); err == nil {
			return resolved, nil
		}
	}
	return r.svc.deps.Models.Resolve(ctx, llmmodel.RoleChatFast)
}

// rewriteIfEmpty is R9: when kb_search found nothing, rewrite the query once with chat.fast and search again.
// A deleted KB must reach presentTool so the next model step has no KB tools; other failures keep the first answer.
func (r *run) rewriteIfEmpty(ctx context.Context, input, data json.RawMessage) (json.RawMessage, error) {
	object, query, ok := rewritableSearch(input, data)
	if !ok {
		return data, nil
	}
	fast := r.fastModel(ctx)
	if fast == nil {
		return data, nil
	}
	started := r.svc.deps.Now()
	rewritten, usage, err := r.rewriteQuery(ctx, fast, query)
	if rewritten == "" || rewritten == query || utf8.RuneCountInString(rewritten) > rewriteMaxRunes {
		r.recordStep(stepKindRewrite, string(ragx.ToolKBSearch), outcomeSkipped, err, usage, started)
		return data, nil
	}
	r.recordStep(stepKindRewrite, string(ragx.ToolKBSearch), outcomeSuccess, nil, usage, started)
	r.status(model.StatusRetryingSearch)
	object[queryField] = rewritten
	retried, err := json.Marshal(object)
	if err != nil {
		return data, nil
	}
	again, err := r.callRagx(ctx, ragx.ToolKBSearch, retried)
	if err != nil {
		if stderrors.Is(err, errors.ErrKbNotFound) {
			return nil, err
		}
		return data, nil
	}
	return again, nil
}

// rewriteQuery asks chat.fast for another query; a failure is returned with an empty rewrite.
func (r *run) rewriteQuery(ctx context.Context, fast *genkitx.ChatModel, query string) (string, genkitx.Usage, error) {
	started := r.svc.deps.Now()
	opts := append(fast.Options(rewriteOutputTokens), ai.WithMessages(ai.NewUserTextMessage(rewritePrompt+query)))
	resp, err := genkit.Generate(ctx, fast.G, opts...)
	usage := genkitx.UsageOf(resp)
	err = genkitx.ToAppError(err)
	r.logLLM(llmmodel.RoleChatFast, r.fastResolved, usage, started, finishReason(resp), err)
	r.addUsage(usage)
	if err != nil {
		return "", usage, err
	}
	return strings.TrimSpace(resp.Text()), usage, nil
}

// rewritableSearch reports whether a kb_search found nothing and had a query to rewrite, and returns its input object.
func rewritableSearch(input, data json.RawMessage) (map[string]any, string, bool) {
	var flag searchFlag
	if json.Unmarshal(data, &flag) != nil || !flag.InsufficientEvidence {
		return nil, "", false
	}
	var object map[string]any
	if json.Unmarshal(input, &object) != nil {
		return nil, "", false
	}
	query, _ := object[queryField].(string)
	return object, query, query != ""
}
