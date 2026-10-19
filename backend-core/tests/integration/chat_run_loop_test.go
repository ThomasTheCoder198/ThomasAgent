package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	agentrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	chatmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/runstream"
	chatservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/service"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx/genkitxtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/jobs"
)

// repoAgents reads the seeded Agent straight from its repository and names its KB, as AgentService.Get does.
type repoAgents struct {
	pool *pgxpool.Pool
}

func (r repoAgents) Get(ctx context.Context, id uuid.UUID) (agentmodel.Agent, error) {
	agent, err := agentrepository.NewAgentRepo().GetAgent(ctx, r.pool, id)
	agent.KBName = "Chính sách"
	return agent, err
}

type chatOnlyModels struct{ model llmmodel.ResolvedModel }

func (m chatOnlyModels) Resolve(context.Context, llmmodel.Role) (llmmodel.ResolvedModel, error) {
	return m.model, nil
}

func (m chatOnlyModels) ResolveModel(context.Context, uuid.UUID) (llmmodel.ResolvedModel, error) {
	return m.model, nil
}

const ragxSearchBody = `{"data":{"results":[{"source":1,"chunkId":"c-1","fileId":"2b1e7c3a-0000-4000-8000-000000000002",` +
	`"fileName":"policy.pdf","fileVersion":3,"nodeId":"9b2f0c1e-0000-4000-8000-000000000001","section":["Chính sách"],` +
	`"pageStart":1,"pageEnd":1,"score":0.9,"blocks":[{"label":"[1.1]","blockId":"b-1","text":"Hoàn tiền trong 7 ngày.",` +
	`"locator":{"kind":"pdf_bbox","page":1}}]}],"insufficientEvidence":false},"meta":{"requestId":"x"}}`

// TestChatRunLoop_ARunBegunByTheAPIAnswersThroughTheWorkerWithOneRequestID runs the real run loop in the real worker
// handler against the real ragx caller (a fake ragx that verifies the App token like ragx does) and a scripted
// provider: BeginRun, relay, claim, a kb_search tool call, an answer with a source, FinishRun, terminal event. The
// request id of the chat request must reach ragx as X-Request-Id and the token's rid, and must never reach the
// provider (PW-2b4, rule 6).
func TestChatRunLoop_ARunBegunByTheAPIAnswersThroughTheWorkerWithOneRequestID(t *testing.T) {
	f := newWorkerFixture(t, nil)
	provider := genkitxtest.NewOpenAIServer(t,
		genkitxtest.Turn{ToolCalls: []genkitxtest.ToolCall{{ID: "call_1", Name: "kb_search", Arguments: `{"query":"hoàn tiền","kbId":"other","orgId":"org-b"}`}}, InputTokens: 50, OutputTokens: 5},
		genkitxtest.Turn{Text: "Được hoàn tiền trong 7 ngày [1].", InputTokens: 120, OutputTokens: 12},
	)
	ragxServer := newFakeRagx(t)
	ragxServer.setResponder(ragxJSON(http.StatusOK, ragxSearchBody))
	model := llmmodel.ResolvedModel{
		ProviderID: uuid.New(), ProviderKind: llmmodel.KindOpenRouter, BaseURL: provider.URL + "/api/v1", APIKey: "sk-test", KeyVersion: "v1",
		ModelRef: "test/chat", Capabilities: []llmmodel.Capability{llmmodel.CapabilityChat, llmmodel.CapabilityTools},
	}
	callLog := calllog.New(logger(t, f.logs), nil, config.LLMLogConfig{})
	run := newRunServiceUnderTest(t, f, repoAgents{pool: f.pool}, chatOnlyModels{model: model}, ragxServer.caller(t, f.logs), callLog)
	f.handler.Executor = run

	ctx, requestID := withRequestID(f.ctx)
	started, outboxID, err := f.store.BeginRun(ctx, submitInput(f.conv, f.agent, "u-1", "Chính sách hoàn tiền?", "a-1"))
	require.NoError(t, err)
	relay := &jobs.OutboxRelay{Pool: f.pool, Redis: f.redis, BatchSize: 10, Log: logger(t, f.logs)}
	require.NoError(t, relay.PublishOne(context.Background(), outboxID))
	require.NoError(t, f.chatRunsConsumer(t).PollOnce(context.Background(), f.handler.Handle))

	// The run ended completed, with the model and token totals, and its steps are recorded.
	var status, modelRef string
	var inputTokens, outputTokens int64
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT status, model_ref, input_tokens, output_tokens FROM runs WHERE id = $1`, started.ID).
		Scan(&status, &modelRef, &inputTokens, &outputTokens))
	require.Equal(t, "completed", status)
	require.Equal(t, "test/chat", modelRef)
	require.EqualValues(t, 170, inputTokens)
	require.EqualValues(t, 17, outputTokens)
	require.Equal(t, 3, countRows(t, f.pool, `SELECT count(*) FROM run_steps WHERE run_id = $1`, started.ID), "model, tool, model")
	var parts string
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT parts::text FROM messages WHERE id = 'a-1' AND role = 'assistant'`).Scan(&parts))
	require.Contains(t, parts, "source-document")
	require.Contains(t, parts, "Được hoàn tiền trong 7 ngày [1].")

	// ragx saw one signed tool call: the run's request id, the org of the run, the Agent's KB, and no model-chosen scope.
	calls := ragxServer.calls()
	require.Len(t, calls, 1)
	require.Equal(t, "/v1/tools/kb_search", calls[0].path)
	require.Equal(t, requestID, calls[0].header.Get("X-Request-Id"), "the run's request id reaches ragx")
	require.Equal(t, requestID, calls[0].claims["rid"])
	require.Equal(t, testOrgID, calls[0].claims["oid"])
	require.Equal(t, "tool:kb_search", calls[0].claims["scp"])
	agent, err := agentrepository.NewAgentRepo().GetAgent(f.ctx, f.pool, f.agent)
	require.NoError(t, err)
	require.Equal(t, agent.KBID.String(), calls[0].claims["kb"], "the KB comes from the Agent, not from the model")
	require.JSONEq(t, `{"query":"hoàn tiền"}`, string(calls[0].body))

	// The provider never saw the request id (rule 6).
	require.Len(t, provider.Requests(), 2)
	for _, request := range provider.Requests() {
		require.Empty(t, request.Header.Get("X-Request-Id"))
	}

	// Every event of the run stream and every log line of the run carries the request id.
	events := f.streamEvents(t, started.ID)
	require.IsType(t, chatmodel.Start{}, events[0])
	require.Equal(t, chatmodel.Finish{}, events[len(events)-1])
	var sourceVersions []int
	for _, event := range events {
		if source, ok := event.(chatmodel.SourceDocument); ok {
			sourceVersions = append(sourceVersions, source.Locator.FileVersion)
		}
	}
	require.Equal(t, []int{3}, sourceVersions, "the citation names the version the blocks belong to")
	for _, entry := range f.streamEntries(t, started.ID) {
		require.Equal(t, requestID, entry.Values[runstream.FieldRequestID])
	}
	for _, msg := range []string{logRunClaimed, logRunFinished, "chat run loop finished", "llm.call", "tool.call"} {
		records := f.logs.find(t, msg)
		require.NotEmpty(t, records, msg)
		for _, record := range records {
			require.Equal(t, requestID, record["request_id"], msg)
		}
	}
	require.Len(t, f.logs.find(t, "llm.call"), 2)
	require.False(t, strings.Contains(f.logs.buf.String(), "sk-test"), "the provider key is never logged")
}

func newRunServiceUnderTest(t *testing.T, f *workerFixture, agents chatservice.AgentReader, models chatservice.ModelResolver, tools chatservice.ToolCaller, calls chatservice.CallLogger) chatservice.RunService {
	t.Helper()
	cfg := f.cfg
	cfg.MaxSteps, cfg.HistoryTurns, cfg.ToolOutputMaxChars = 6, 20, 32000
	cfg.MaxParallelTools, cfg.MaxToolCallsPerStep, cfg.MaxOutputTokens, cfg.DefaultContextWindow = 4, 8, 512, 128000
	service, err := chatservice.NewRunService(chatservice.RunDeps{
		Store: f.store, Agents: agents, Models: models, Registry: genkitx.NewModelRegistry(genkitx.Config{CacheSize: 4}),
		Tools: tools, Calls: calls, Config: cfg, ToolTimeout: 5 * time.Second, Log: logger(t, f.logs),
	})
	require.NoError(t, err)
	return service
}
