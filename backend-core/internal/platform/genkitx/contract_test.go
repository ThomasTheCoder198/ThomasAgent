package genkitx_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/firebase/genkit/go/plugins/compat_oai/openrouter"
	systemonex "github.com/firebase/genkit/go/plugins/systemone/exp"
	"github.com/openai/openai-go/option"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx/genkitxtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

// These tests pin the Genkit behaviour the M2 run loop relies on (plan 2b-1). Keep them green on every Genkit bump.

const (
	pluginPlaceholderKey = "placeholder-plugin-key"
	requestKey           = "sk-request-key"
	decisionKey          = "sk-decision-key"
	chatModel            = "openrouter/test/model"
	decisionModel        = "openrouter-decisions/typesafe/jev-1.14"
	openRouterAPIPath    = "/api/v1"
	decisionsPath        = "/api/alpha/decisions"
	headerAuthorization  = "Authorization"
	headerRequestID      = "X-Request-Id"
	testRequestID        = "req_contract-0000-0000"
)

// kbSearchSchema mirrors how 2b-4 builds tools: an unregistered tool from a plain JSON Schema.
var kbSearchSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []any{"query"},
	"properties": map[string]any{"query": map[string]any{"type": "string", "minLength": 1}},
}

func kbSearchTool(run func()) ai.ToolRef {
	return ai.NewTool[any, any]("kb_search", "Search the knowledge base",
		func(*ai.ToolContext, any) (any, error) {
			run()
			return nil, nil
		}, ai.WithInputSchema(kbSearchSchema))
}

func openRouterGenkit(ctx context.Context, server *genkitxtest.OpenAIServer) *genkit.Genkit {
	return genkit.Init(ctx, genkit.WithPlugins(&openrouter.OpenRouter{
		APIKey: pluginPlaceholderKey,
		Opts:   []option.RequestOption{option.WithBaseURL(server.URL + openRouterAPIPath), option.WithMaxRetries(0)},
	}))
}

func perRequest(key string) ai.ConfigOption {
	return ai.WithConfig(openrouter.ChatConfig{RequestConfig: compat_oai.RequestConfig{APIKey: key}})
}

func TestContract_PerRequestKeyWinsAndNoRequestIDLeaves(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), testRequestID)
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "OK", InputTokens: 3, OutputTokens: 1})
	g := openRouterGenkit(ctx, server)

	text, err := genkit.GenerateText(ctx, g, ai.WithModelName(chatModel), perRequest(requestKey),
		ai.WithMessages(ai.NewUserTextMessage("Reply with OK")))

	require.NoError(t, err)
	require.Equal(t, "OK", text)
	requests := server.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, "Bearer "+requestKey, requests[0].Header.Get(headerAuthorization))
	require.Empty(t, requests[0].Header.Get(headerRequestID))
}

func TestContract_ReturnToolRequestsStreamsReasoningAndLeavesToolsToTheCaller(t *testing.T) {
	ctx := context.Background()
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{
		Reasoning:   "Cần tìm trong KB",
		ToolCalls:   []genkitxtest.ToolCall{{ID: "call_1", Name: "kb_search", Arguments: `{"query":"hoàn tiền"}`}},
		InputTokens: 11, OutputTokens: 7,
	})
	g := openRouterGenkit(ctx, server)
	var executed atomic.Bool
	tool := kbSearchTool(func() { executed.Store(true) })

	var reasoning strings.Builder
	var final *ai.ModelResponse
	for value, err := range genkit.GenerateStream(ctx, g, ai.WithModelName(chatModel), perRequest(requestKey),
		ai.WithMessages(ai.NewUserTextMessage("Chính sách hoàn tiền?")), ai.WithTools(tool),
		ai.WithReturnToolRequests(true)) {
		require.NoError(t, err)
		if value.Done {
			final = value.Response
			break
		}
		for _, part := range value.Chunk.Content {
			if part.IsReasoning() {
				reasoning.WriteString(part.Text)
			}
		}
	}

	require.False(t, executed.Load(), "Genkit must return the tool request, not run it")
	require.Equal(t, "Cần tìm trong KB", reasoning.String())
	require.NotNil(t, final)
	requests := final.ToolRequests()
	require.Len(t, requests, 1)
	require.Equal(t, "kb_search", requests[0].ToolRequest.Name)
	require.Equal(t, "call_1", requests[0].ToolRequest.Ref)
	require.Equal(t, map[string]any{"query": "hoàn tiền"}, requests[0].ToolRequest.Input)
	require.NotNil(t, final.Usage)
	require.Equal(t, 11, final.Usage.InputTokens)
	require.Equal(t, 7, final.Usage.OutputTokens)
	sent := server.Requests()[0].Body["tools"].([]any)
	function := sent[0].(map[string]any)["function"].(map[string]any)
	require.Equal(t, "kb_search", function["name"])
	require.Equal(t, false, function["parameters"].(map[string]any)["additionalProperties"],
		"the schema reaches the provider as given")
}

func TestContract_ToolResponseGoesBackAsAToolMessage(t *testing.T) {
	ctx := context.Background()
	server := genkitxtest.NewOpenAIServer(t,
		genkitxtest.Turn{ToolCalls: []genkitxtest.ToolCall{{ID: "call_1", Name: "kb_search", Arguments: `{"query":"x"}`}}},
		genkitxtest.Turn{Text: "Không tìm thấy thông tin phù hợp."},
	)
	g := openRouterGenkit(ctx, server)
	tool := kbSearchTool(func() {})
	user := ai.NewUserTextMessage("x?")
	first, err := genkit.Generate(ctx, g, ai.WithModelName(chatModel), perRequest(requestKey),
		ai.WithMessages(user), ai.WithTools(tool), ai.WithReturnToolRequests(true))
	require.NoError(t, err)
	call := first.ToolRequests()[0].ToolRequest
	toolMessage := ai.NewMessage(ai.RoleTool, nil, ai.NewToolResponsePart(&ai.ToolResponse{
		Name: call.Name, Ref: call.Ref, Output: map[string]any{"results": []any{}, "insufficientEvidence": true},
	}))

	second, err := genkit.Generate(ctx, g, ai.WithModelName(chatModel), perRequest(requestKey),
		ai.WithMessages(user, first.Message, toolMessage), ai.WithTools(tool), ai.WithReturnToolRequests(true))

	require.NoError(t, err)
	require.Equal(t, "Không tìm thấy thông tin phù hợp.", second.Text())
	messages := server.Requests()[1].Body["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	require.Equal(t, "tool", last["role"])
	require.Equal(t, "call_1", last["tool_call_id"])
	assistant := messages[len(messages)-2].(map[string]any)
	require.Equal(t, "assistant", assistant["role"])
	calls := assistant["tool_calls"].([]any)
	require.Equal(t, "call_1", calls[0].(map[string]any)["id"])
	var output map[string]any
	require.NoError(t, json.Unmarshal([]byte(last["content"].(string)), &output))
	require.Equal(t, true, output["insufficientEvidence"])
	require.Equal(t, []any{}, output["results"])
}

type route string

func (route) Criteria() map[route]string {
	return map[route]string{"smalltalk": "Greeting or chit-chat", "kb": "Needs the knowledge base"}
}

type verdict struct {
	Route     systemonex.Choice[route] `json:"route" jsonschema_description:"Which path serves this message?"`
	Supported systemonex.Noul          `json:"supported" jsonschema_description:"Is the claim supported?"`
}

func TestContract_SystemOneOverOpenRouterDecisions(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), testRequestID)
	server := genkitxtest.NewDecisionsServer(t, map[string]any{
		"route": map[string]any{"type": "choice", "choice": "smalltalk", "confidence": 0.93,
			"probabilities": map[string]any{"smalltalk": 0.93, "kb": 0.07}},
		"supported": map[string]any{"type": "noul", "noul": 0.8},
	})
	plugin := systemonex.OpenRouter()
	plugin.APIKey = decisionKey
	plugin.BaseURL = server.URL
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	out, _, err := genkit.GenerateData[verdict](ctx, g, ai.WithModelName(decisionModel),
		ai.WithMessages(ai.NewUserTextMessage("xin chào")))

	require.NoError(t, err)
	require.Equal(t, route("smalltalk"), out.Route.Choice)
	require.InDelta(t, 0.93, out.Route.Confidence, 1e-9)
	require.InDelta(t, 0.8, out.Supported.Probability, 1e-9)
	requests := server.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, decisionsPath, requests[0].Path)
	require.Equal(t, "Bearer "+decisionKey, requests[0].Header.Get(headerAuthorization))
	require.Empty(t, requests[0].Header.Get(headerRequestID))
	questions := requests[0].Body["questions"].(map[string]any)
	require.Contains(t, questions, "route")
	require.Contains(t, questions, "supported")
}
