package calllog

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

const (
	eventLLMCall  = "llm.call"
	eventToolCall = "tool.call"

	fieldRunID           = "run_id"
	fieldStep            = "step"
	fieldProvider        = "provider"
	fieldModel           = "model"
	fieldRole            = "role"
	fieldInputTokens     = "input_tokens"
	fieldOutputTokens    = "output_tokens"
	fieldCachedTokens    = "cached_tokens"
	fieldReasoningTokens = "reasoning_tokens"
	fieldCost            = "cost"
	fieldTTFTMS          = "ttft_ms"
	fieldAttempt         = "attempt"
	fieldFinishReason    = "finish_reason"
	fieldErrorCode       = "error_code"
	fieldPrompt          = "prompt"
	fieldResponse        = "response"
	fieldToolArgs        = "tool_args"
	fieldToolResult      = "tool_result"
	fieldTruncated       = "truncated"
	fieldApp             = "app"
	fieldTool            = "tool"
	fieldOutcome         = "outcome"
	fieldRagxTraceID     = "ragx_trace_id"
)

// LLMRecorder is the part of the metrics registry this package needs.
type LLMRecorder interface {
	ObserveLLMCall(metrics.LLMObservation)
}

type LLMContent struct {
	Prompt, Response, ToolArgs, ToolResult string
}

type LLMCall struct {
	RunID                                                    string
	Step                                                     int
	Provider, Model, Role                                    string
	InputTokens, OutputTokens, CachedTokens, ReasoningTokens int64
	CostUSD                                                  float64
	Duration, TTFT                                           time.Duration
	Attempt                                                  int
	FinishReason, ErrorCode                                  string
	WillRetry                                                bool
	Content                                                  LLMContent
}

type ToolCall struct {
	App, Tool              string
	Duration               time.Duration
	ErrorCode, RagxTraceID string
}

// Logger writes one event per model or tool call; the shared logger adds request_id and redacts secrets.
type Logger struct {
	log      *slog.Logger
	recorder LLMRecorder
	cfg      config.LLMLogConfig
}

func New(log *slog.Logger, recorder LLMRecorder, cfg config.LLMLogConfig) *Logger {
	return &Logger{log: log, recorder: recorder, cfg: cfg}
}

func (l *Logger) LLM(ctx context.Context, call LLMCall) {
	outcome := llmOutcome(call)
	attrs := []slog.Attr{
		slog.String(fieldRunID, call.RunID), slog.Int(fieldStep, call.Step),
		slog.String(fieldProvider, call.Provider), slog.String(fieldModel, call.Model), slog.String(fieldRole, call.Role),
		slog.Int64(fieldInputTokens, call.InputTokens), slog.Int64(fieldOutputTokens, call.OutputTokens),
		slog.Int64(fieldCachedTokens, call.CachedTokens), slog.Int64(fieldReasoningTokens, call.ReasoningTokens),
		slog.Float64(fieldCost, call.CostUSD), slog.Int64(logging.FieldDurationMS, call.Duration.Milliseconds()),
		slog.Int64(fieldTTFTMS, call.TTFT.Milliseconds()), slog.Int(fieldAttempt, call.Attempt),
		slog.String(fieldFinishReason, call.FinishReason), slog.String(fieldOutcome, outcome),
	}
	if call.ErrorCode != "" {
		attrs = append(attrs, slog.String(fieldErrorCode, call.ErrorCode))
	}
	if l.cfg.Content {
		attrs = append(attrs, l.contentAttrs(call.Content)...)
	}
	l.log.LogAttrs(ctx, levelFor(outcome), eventLLMCall, attrs...)
	if l.recorder != nil {
		l.recorder.ObserveLLMCall(metrics.LLMObservation{
			Provider: call.Provider, Model: call.Model, Role: call.Role, Outcome: outcome,
			InputTokens: call.InputTokens, OutputTokens: call.OutputTokens, CachedTokens: call.CachedTokens, ReasoningTokens: call.ReasoningTokens,
			CostUSD: call.CostUSD, Duration: call.Duration, TTFT: call.TTFT,
		})
	}
}

func (l *Logger) Tool(ctx context.Context, call ToolCall) {
	outcome := metrics.OutcomeSuccess
	if call.ErrorCode != "" {
		outcome = metrics.OutcomeError
	}
	attrs := []slog.Attr{
		slog.String(fieldApp, call.App), slog.String(fieldTool, call.Tool),
		slog.Int64(logging.FieldDurationMS, call.Duration.Milliseconds()), slog.String(fieldOutcome, outcome),
	}
	if call.ErrorCode != "" {
		attrs = append(attrs, slog.String(fieldErrorCode, call.ErrorCode))
	}
	if outbound.IsRagxTraceID(call.RagxTraceID) {
		attrs = append(attrs, slog.String(fieldRagxTraceID, call.RagxTraceID))
	}
	level := slog.LevelInfo
	if outcome == metrics.OutcomeError {
		level = slog.LevelWarn
	}
	l.log.LogAttrs(ctx, level, eventToolCall, attrs...)
}

func (l *Logger) contentAttrs(content LLMContent) []slog.Attr {
	var attrs []slog.Attr
	truncated := false
	for _, field := range []struct{ key, value string }{
		{fieldPrompt, content.Prompt}, {fieldResponse, content.Response},
		{fieldToolArgs, content.ToolArgs}, {fieldToolResult, content.ToolResult},
	} {
		if field.value == "" {
			continue
		}
		redacted := redactContent(field.key, field.value)
		value, cut := truncateUTF8(redacted, l.cfg.ContentMaxBytes)
		truncated = truncated || cut
		attrs = append(attrs, slog.String(field.key, value))
	}
	if truncated {
		attrs = append(attrs, slog.Bool(fieldTruncated, true))
	}
	return attrs
}

func redactContent(key, value string) string {
	if !json.Valid([]byte(value)) {
		return logging.RedactValue(key, slog.StringValue(value)).String()
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var structured any
	if err := decoder.Decode(&structured); err != nil {
		return logging.RedactValue(key, slog.StringValue(value)).String()
	}
	redacted := logging.RedactValue(key, slog.AnyValue(structured))
	encoded, err := json.Marshal(redacted.Any())
	if err != nil {
		return logging.RedactValue(key, slog.StringValue(value)).String()
	}
	return string(encoded)
}

func llmOutcome(call LLMCall) string {
	switch {
	case call.ErrorCode == "":
		return metrics.OutcomeSuccess
	case call.WillRetry:
		return metrics.OutcomeRetry
	default:
		return metrics.OutcomeError
	}
}

func levelFor(outcome string) slog.Level {
	switch outcome {
	case metrics.OutcomeRetry:
		return slog.LevelWarn
	case metrics.OutcomeError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// truncateUTF8 keeps at most limit bytes without splitting a character.
func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut], true
}
