package calllog

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
)

const testRequestID = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"

type fakeLLMRecorder struct{ observations []metrics.LLMObservation }

func (f *fakeLLMRecorder) ObserveLLMCall(o metrics.LLMObservation) {
	f.observations = append(f.observations, o)
}

func newTestLogger(t *testing.T, cfg config.LLMLogConfig) (*Logger, *bytes.Buffer, *fakeLLMRecorder) {
	t.Helper()
	var output bytes.Buffer
	log, err := logging.NewLogger(&output, "debug", "core-test", "test", nil)
	require.NoError(t, err)
	recorder := &fakeLLMRecorder{}
	return New(log, recorder, cfg), &output, recorder
}

func lastRecord(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	var record map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &record))
	return record
}

func sampleCall() LLMCall {
	return LLMCall{
		RunID: "run-1", Step: 2, Provider: "openrouter", Model: "anthropic/claude-sonnet-4.5", Role: "chat.default",
		InputTokens: 100, OutputTokens: 20, CachedTokens: 50, CostUSD: 0.01, Duration: 2 * time.Second,
		TTFT: 400 * time.Millisecond, Attempt: 1, FinishReason: "stop",
		Content: LLMContent{Prompt: "hello sk-abcdef123456 world", Response: "answer"},
	}
}

func TestLLM_LogsMetadataWithRequestIDAndRecordsMetrics(t *testing.T) {
	l, output, recorder := newTestLogger(t, config.LLMLogConfig{ContentMaxBytes: 64})
	l.LLM(logging.ContextWithRequestID(context.Background(), testRequestID), sampleCall())
	record := lastRecord(t, output)
	require.Equal(t, "llm.call", record["msg"])
	require.Equal(t, "INFO", record["level"])
	require.Equal(t, testRequestID, record["request_id"])
	require.Equal(t, "openrouter", record["provider"])
	require.EqualValues(t, 100, record["input_tokens"])
	require.EqualValues(t, 2000, record["duration_ms"])
	require.EqualValues(t, 400, record["ttft_ms"])
	require.Len(t, recorder.observations, 1)
	require.Equal(t, metrics.OutcomeSuccess, recorder.observations[0].Outcome)
}

func TestLLM_ContentOffByDefault(t *testing.T) {
	l, output, _ := newTestLogger(t, config.LLMLogConfig{ContentMaxBytes: 64})
	l.LLM(context.Background(), sampleCall())
	require.NotContains(t, output.String(), "hello")
	require.NotContains(t, output.String(), "answer")
	require.NotContains(t, lastRecord(t, output), "prompt")
}

func TestLLM_ContentIsRedactedAndTruncatedOnRuneBoundary(t *testing.T) {
	l, output, _ := newTestLogger(t, config.LLMLogConfig{Content: true, ContentMaxBytes: 10})
	call := sampleCall()
	call.Content = LLMContent{Prompt: "sk-abcdef123456", Response: "ááááááááá"} // á is 2 bytes
	originalResponse := call.Content.Response
	l.LLM(context.Background(), call)
	record := lastRecord(t, output)
	require.NotContains(t, output.String(), "abcdef123456")
	response, _ := record["response"].(string)
	require.LessOrEqual(t, len(response), 10)
	require.True(t, strings.HasPrefix(originalResponse, response), "cut must not split a multi-byte character")
	require.Equal(t, true, record["truncated"])
}

func TestLLM_LevelFollowsOutcome(t *testing.T) {
	for _, tc := range []struct {
		errorCode string
		retry     bool
		level     string
		outcome   string
	}{
		{"", false, "INFO", metrics.OutcomeSuccess},
		{"PROVIDER_UNAVAILABLE", true, "WARN", metrics.OutcomeRetry},
		{"PROVIDER_UNAVAILABLE", false, "ERROR", metrics.OutcomeError},
	} {
		l, output, recorder := newTestLogger(t, config.LLMLogConfig{ContentMaxBytes: 64})
		call := sampleCall()
		call.ErrorCode, call.WillRetry = tc.errorCode, tc.retry
		l.LLM(context.Background(), call)
		require.Equal(t, tc.level, lastRecord(t, output)["level"])
		require.Equal(t, tc.outcome, recorder.observations[0].Outcome)
	}
}

func TestTool_LogsCallWithRagxTraceID(t *testing.T) {
	l, output, _ := newTestLogger(t, config.LLMLogConfig{ContentMaxBytes: 64})
	l.Tool(logging.ContextWithRequestID(context.Background(), testRequestID), ToolCall{
		App: "ragx", Tool: "kb.getKnowledgebase", Duration: 150 * time.Millisecond,
		RagxTraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	record := lastRecord(t, output)
	require.Equal(t, "tool.call", record["msg"])
	require.Equal(t, testRequestID, record["request_id"])
	require.Equal(t, "kb.getKnowledgebase", record["tool"])
	require.Equal(t, metrics.OutcomeSuccess, record["outcome"])
	require.EqualValues(t, 150, record["duration_ms"])
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", record["ragx_trace_id"])
}

func TestTool_DropsMalformedRagxTraceID(t *testing.T) {
	l, output, _ := newTestLogger(t, config.LLMLogConfig{ContentMaxBytes: 64})
	l.Tool(context.Background(), ToolCall{App: "ragx", Tool: "kb.search", ErrorCode: "TOOL_FAILED", RagxTraceID: "not-a-trace\n{}"})
	record := lastRecord(t, output)
	require.NotContains(t, record, "ragx_trace_id")
	require.Equal(t, "WARN", record["level"])
	require.Equal(t, metrics.OutcomeError, record["outcome"])
}

func TestLLM_RedactsBeforeApplyingContentLimit(t *testing.T) {
	logger, output, _ := newTestLogger(t, config.LLMLogConfig{Content: true, ContentMaxBytes: 5})
	call := sampleCall()
	call.Content = LLMContent{Prompt: "sk-abcdef123456"}
	logger.LLM(context.Background(), call)
	record := lastRecord(t, output)
	require.Equal(t, "[REDA", record["prompt"])
	require.Equal(t, true, record["truncated"])
}

func TestLLM_RedactedContentRespectsByteLimit(t *testing.T) {
	logger, output, _ := newTestLogger(t, config.LLMLogConfig{Content: true, ContentMaxBytes: 12})
	call := sampleCall()
	call.Content = LLMContent{Prompt: "abc sk-abcdef123456"}
	logger.LLM(context.Background(), call)
	prompt, _ := lastRecord(t, output)["prompt"].(string)
	require.LessOrEqual(t, len(prompt), 12)
	require.NotContains(t, prompt, "sk-")
}

func TestLLM_RedactsSerializedStructuredSecrets(t *testing.T) {
	logger, output, _ := newTestLogger(t, config.LLMLogConfig{Content: true, ContentMaxBytes: 1024})
	call := sampleCall()
	call.Content = LLMContent{
		ToolArgs:   `{"password":"ordinary-password","api_key":"ordinary-key","count":9007199254740993}`,
		ToolResult: `[{"headers":{"Authorization":"ordinary-auth"},"items":[{"token":"ordinary-token","name":"visible"}]}]`,
	}
	logger.LLM(context.Background(), call)
	record := lastRecord(t, output)
	args, _ := record["tool_args"].(string)
	result, _ := record["tool_result"].(string)
	for _, secret := range []string{"ordinary-password", "ordinary-key", "ordinary-auth", "ordinary-token"} {
		require.NotContains(t, output.String(), secret)
	}
	require.Contains(t, args, `"password":"[REDACTED]"`)
	require.Contains(t, args, `"api_key":"[REDACTED]"`)
	require.Contains(t, args, "9007199254740993")
	require.Contains(t, result, `"name":"visible"`)
}

func TestLLM_RedactsCamelCaseStructuredSecrets(t *testing.T) {
	logger, output, _ := newTestLogger(t, config.LLMLogConfig{Content: true, ContentMaxBytes: 1024})
	call := sampleCall()
	call.Content = LLMContent{
		ToolArgs:   `{"apiKey":"api-value","APIKey":"acronym-value","privateKey":"private-value"}`,
		ToolResult: `[{"accessToken":"oauth-value","refreshToken":"refresh-value","clientSecret":"client-value","inputTokens":392,"tokenCount":412}]`,
	}
	logger.LLM(context.Background(), call)
	for _, secret := range []string{"api-value", "acronym-value", "private-value", "oauth-value", "refresh-value", "client-value"} {
		require.NotContains(t, output.String(), secret)
	}
	record := lastRecord(t, output)
	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(record["tool_args"].(string)), &args))
	for _, key := range []string{"apiKey", "APIKey", "privateKey"} {
		require.Equal(t, logging.RedactedPlaceholder, args[key])
	}
	var result []map[string]any
	require.NoError(t, json.Unmarshal([]byte(record["tool_result"].(string)), &result))
	for _, key := range []string{"accessToken", "refreshToken", "clientSecret"} {
		require.Equal(t, logging.RedactedPlaceholder, result[0][key])
	}
	require.EqualValues(t, 392, result[0]["inputTokens"])
	require.EqualValues(t, 412, result[0]["tokenCount"])
}
