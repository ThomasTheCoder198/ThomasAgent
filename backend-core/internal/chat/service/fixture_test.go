package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/worker"
	convmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx/genkitxtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

var (
	testKBID           = uuid.MustParse("3c2f8d4b-0000-4000-8000-000000000003")
	testAgentID        = uuid.MustParse("4d3a9e5c-0000-4000-8000-000000000004")
	testUserID         = uuid.MustParse("5e4baf6d-0000-4000-8000-000000000005")
	testConversationID = uuid.MustParse("6f5cb07e-0000-4000-8000-000000000006")
)

const (
	testRequestID      = "req_run-loop-test"
	testAssistantID    = "a-run-1"
	saveTimeoutForTest = 50 * time.Millisecond // short so the hung-write test finishes quickly
	testFileVersion    = 2                     // the ingested version searchData's results report (Codex round 2 #4)
)

func testChatConfig() config.ChatConfig {
	return config.ChatConfig{
		MaxSteps: 6, RunDeadline: 10 * time.Second, HistoryTurns: 20, CompactionRatio: 0.7, CompactionKeepTurns: 4, CompactionMaxPages: 4,
		RouterThreshold: 0.85, CitationFlagBelow: 0.5, CitationMaxPairs: 16, ToolOutputMaxChars: 32000,
		TitleMaxRunes: 60, MessageMaxRunes: 8000, SteerMaxPending: 5, SteerMaxRunes: 2000, DefaultContextWindow: 128000,
		FinishTimeout: 5 * time.Second, MaxParallelTools: 4, MaxToolCallsPerStep: 8, MaxOutputTokens: 512,
		TitleMaxOutputTokens: 32, SummaryMaxOutputTokens: 256,
	}
}

// memoryStore is an in-memory RunStore with the semantics of ChatStoreImpl: messages in seq order, fenced writes.
type memoryStore struct {
	mu        sync.Mutex
	messages  []convmodel.Message
	steps     []convmodel.RunStep
	finished  []convmodel.Run
	assistant []convmodel.Message
	finishes  int
	summary   string
	peak      int64
	// finishErr, when set, fails FinishRun for the messages it returns an error for (save-failure tests). It gets
	// the call's context so a test can block until the write times out.
	finishErr func(ctx context.Context, assistant convmodel.Message) error
	// stepErr, when set, fails RecordStep (a lost lease is errors.ErrRunLeaseLost).
	stepErr func(step convmodel.RunStep) error
}

func (s *memoryStore) LoadHistory(_ context.Context, conversationID uuid.UUID, _ int) (convmodel.History, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv := convmodel.Conversation{ID: conversationID, AgentID: testAgentID, UserID: testUserID, Summary: s.summary}
	return convmodel.History{Conversation: conv, Messages: append([]convmodel.Message(nil), s.messages...), PeakInputTokens: s.peak}, nil
}

func (s *memoryStore) RecordStep(_ context.Context, _ convmodel.Lease, step convmodel.RunStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stepErr != nil {
		if err := s.stepErr(step); err != nil {
			return err
		}
	}
	s.steps = append(s.steps, step)
	return nil
}

// FinishRun fails on a done context, as a pgx write does, so a recovery that reuses an expired context fails here.
func (s *memoryStore) FinishRun(ctx context.Context, _ convmodel.Lease, run convmodel.Run, assistant convmodel.Message) error {
	s.mu.Lock()
	s.finishes++
	s.mu.Unlock()
	if s.finishErr != nil {
		if err := s.finishErr(ctx, assistant); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = append(s.finished, run)
	if assistant.ID != "" {
		assistant.Seq = int64(len(s.messages) + 1)
		assistant.Role = "assistant"
		s.messages = append(s.messages, assistant)
		s.assistant = append(s.assistant, assistant)
	}
	return nil
}

func (s *memoryStore) stepKinds() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	kinds := make([]string, 0, len(s.steps))
	for _, step := range s.steps {
		kinds = append(kinds, string(step.Kind))
	}
	return kinds
}

type fakeAgents struct{ agent agentmodel.Agent }

func (f fakeAgents) Get(_ context.Context, id uuid.UUID) (agentmodel.Agent, error) {
	if id != f.agent.ID {
		return agentmodel.Agent{}, errors.ErrAgentNotFound
	}
	return f.agent, nil
}

// fakeModels resolves every role to the fixture; fast and decision are optional.
type fakeModels struct {
	chat     llmmodel.ResolvedModel
	fast     *llmmodel.ResolvedModel
	decision *llmmodel.ResolvedModel
	// byID answers ResolveModel for pinned and requested models; any other id resolves to chat.
	byID map[uuid.UUID]llmmodel.ResolvedModel
}

func (f fakeModels) Resolve(_ context.Context, role llmmodel.Role) (llmmodel.ResolvedModel, error) {
	switch role {
	case llmmodel.RoleChatDefault:
		return f.chat, nil
	case llmmodel.RoleChatFast:
		if f.fast != nil {
			return *f.fast, nil
		}
	case llmmodel.RoleDecision:
		if f.decision != nil {
			return *f.decision, nil
		}
		if f.fast != nil { // spec §3.3 fallback, as ResolveService does it
			fallback := *f.fast
			fallback.Source = llmmodel.ResolvedSourceFallback
			return fallback, nil
		}
	}
	return llmmodel.ResolvedModel{}, errors.ErrLLMRoleNotAssigned
}

func (f fakeModels) ResolveModel(_ context.Context, id uuid.UUID) (llmmodel.ResolvedModel, error) {
	if resolved, ok := f.byID[id]; ok {
		return resolved, nil
	}
	return f.chat, nil
}

type toolCall struct {
	tool  ragx.ToolName
	input map[string]any
}

type fakeTools struct {
	mu     sync.Mutex
	calls  []toolCall
	answer func(n int, tool ragx.ToolName, input map[string]any) (json.RawMessage, error)
}

func (f *fakeTools) CallTool(_ context.Context, kbID uuid.UUID, tool ragx.ToolName, input json.RawMessage) (json.RawMessage, error) {
	if kbID != testKBID {
		return nil, fmt.Errorf("unexpected kb %s", kbID)
	}
	var decoded map[string]any
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.calls = append(f.calls, toolCall{tool: tool, input: decoded})
	n := len(f.calls)
	f.mu.Unlock()
	return f.answer(n, tool, decoded)
}

func (f *fakeTools) seen() []toolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]toolCall(nil), f.calls...)
}

type recordedLLM struct {
	mu    sync.Mutex
	calls []calllog.LLMCall
}

func (r *recordedLLM) LLM(_ context.Context, call calllog.LLMCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

// fakeControl is the run's Stop subscription: the test fires it, or scripts what Check answers.
type fakeControl struct {
	stopped   chan struct{}
	once      sync.Once
	mu        sync.Mutex
	checks    int
	stopAfter int // Check reports a stop from this call on (1-based); 0 = never
}

func newFakeControl() *fakeControl { return &fakeControl{stopped: make(chan struct{})} }

func (c *fakeControl) Stopped() <-chan struct{} { return c.stopped }

func (c *fakeControl) fire() { c.once.Do(func() { close(c.stopped) }) }

func (c *fakeControl) Check(context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks++
	if c.stopAfter > 0 && c.checks >= c.stopAfter {
		c.fire()
		return true, nil
	}
	return false, nil
}

func (c *fakeControl) Close() error { return nil }

// emitSink records what the executor emits, and can fail the nth call (a Redis outage or a stream cap).
type emitSink struct {
	mu     sync.Mutex
	events []model.Event
	calls  int
	// failIf, when set, makes Emit return its error for the events it returns one for.
	failIf func(call int, event model.Event) error
}

func (e *emitSink) emit(event model.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.failIf != nil {
		if err := e.failIf(e.calls, event); err != nil {
			return err
		}
	}
	e.events = append(e.events, event)
	return nil
}

func (e *emitSink) all() []model.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]model.Event(nil), e.events...)
}

type harness struct {
	server  *genkitxtest.OpenAIServer
	store   *memoryStore
	tools   *fakeTools
	llm     *recordedLLM
	control *fakeControl
	sink    *emitSink
	deps    service.RunDeps
	// run is the row the worker claimed; tests adjust it (locale, requested model) before executing.
	run convmodel.Run
}

func fixtureModel(server *genkitxtest.OpenAIServer, capabilities ...llmmodel.Capability) llmmodel.ResolvedModel {
	if len(capabilities) == 0 {
		capabilities = []llmmodel.Capability{llmmodel.CapabilityChat, llmmodel.CapabilityTools}
	}
	return llmmodel.ResolvedModel{
		ProviderID: uuid.New(), ProviderKind: llmmodel.KindOpenRouter, BaseURL: server.URL + "/api/v1",
		APIKey: "sk-test-key", KeyVersion: "v1", ModelRef: "test/chat", Capabilities: capabilities,
	}
}

func newHarness(t *testing.T, configure ...func(*harness)) *harness {
	t.Helper()
	server := genkitxtest.NewOpenAIServer(t)
	h := &harness{server: server, store: &memoryStore{}, tools: &fakeTools{}, llm: &recordedLLM{}, control: newFakeControl(), sink: &emitSink{}}
	h.deps = service.RunDeps{
		Store: h.store,
		Agents: fakeAgents{agent: agentmodel.Agent{ID: testAgentID, Name: "Hỗ trợ", KBID: testKBID, KBName: "Chính sách",
			Instructions: "Trả lời ngắn gọn."}},
		Models:      fakeModels{chat: fixtureModel(server)},
		Registry:    genkitx.NewModelRegistry(genkitx.Config{CacheSize: 4}),
		Tools:       h.tools,
		Calls:       h.llm,
		Config:      testChatConfig(),
		ToolTimeout: 5 * time.Second,
		Log:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	h.run = convmodel.Run{
		ID: uuid.New(), ConversationID: testConversationID, AssistantMessageID: testAssistantID,
		Status: convmodel.RunStatusRunning, DeadlineAt: time.Now().Add(h.deps.Config.RunDeadline),
		RequestID: testRequestID, Locale: convmodel.LocaleVI,
	}
	for _, c := range configure {
		c(h)
	}
	return h
}

func (h *harness) service(t *testing.T) service.RunService {
	t.Helper()
	svc, err := service.NewRunService(h.deps)
	require.NoError(t, err)
	return svc
}

// seed stores the user message BeginRun would have appended.
func (h *harness) seed(text string) {
	parts := fmt.Sprintf(`[{"type":"text","text":%q}]`, text)
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.messages = append(h.store.messages, convmodel.Message{
		ID: "u-" + uuid.NewString(), Role: "user", Seq: int64(len(h.store.messages) + 1), Parts: json.RawMessage(parts),
	})
}

func (h *harness) input() worker.ExecuteInput {
	return worker.ExecuteInput{
		Run: h.run, Lease: convmodel.Lease{RunID: h.run.ID, Token: uuid.New(), Owner: "test"},
		Emit: h.sink.emit, Control: h.control,
	}
}

// execute runs the executor the way the worker handler does and returns what it returned.
func (h *harness) execute(t *testing.T, ctx context.Context, text string) (model.Event, error) {
	t.Helper()
	h.seed(text)
	return h.service(t).Execute(ctx, h.input())
}

// run executes under a plain context and returns the emitted events with the terminal event appended, which is the
// order the browser sees them in.
func (h *harness) runAll(t *testing.T, text string) []model.Event {
	t.Helper()
	terminal, err := h.execute(t, context.Background(), text)
	require.NoError(t, err)
	require.NotNil(t, terminal, "an executor that ends the run returns the terminal event")
	return append(h.sink.all(), terminal)
}

func eventsOf[T model.Event](events []model.Event) []T {
	var out []T
	for _, event := range events {
		if typed, ok := event.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

func answerText(events []model.Event) string {
	var b strings.Builder
	for _, delta := range eventsOf[model.TextDelta](events) {
		b.WriteString(delta.Delta)
	}
	return b.String()
}

type chunkSpec struct {
	chunkID string
	source  int
	text    string
}

// searchData answers like ragx kb_search: numbered results, labelled blocks, one pdf locator each.
func searchData(insufficient bool, chunks ...chunkSpec) json.RawMessage {
	results := make([]map[string]any, 0, len(chunks))
	for _, c := range chunks {
		results = append(results, map[string]any{
			"source": c.source, "chunkId": c.chunkID, "fileId": "2b1e7c3a-0000-4000-8000-000000000002",
			"fileName": "policy.pdf", "fileVersion": testFileVersion, "nodeId": "9b2f0c1e-0000-4000-8000-000000000001",
			"section": []string{"Chính sách", "Hoàn tiền"}, "pageStart": 3, "pageEnd": 3, "score": 0.9,
			"blocks": []map[string]any{{
				"label": fmt.Sprintf("[%d.1]", c.source), "blockId": "b-" + c.chunkID, "text": c.text,
				"locator": map[string]any{"kind": "pdf_bbox", "page": 3},
			}},
		})
	}
	raw, _ := json.Marshal(map[string]any{"results": results, "insufficientEvidence": insufficient})
	return raw
}

// toolMessageText is the text of a tool message as the model reads it. Genkit's OpenAI adapter sends a tool response
// as its JSON encoding, so a string output arrives as a quoted JSON string.
func toolMessageText(t *testing.T, content any) string {
	t.Helper()
	raw, _ := content.(string)
	var text string
	require.NoError(t, json.Unmarshal([]byte(raw), &text), "a tool message carries a JSON string")
	return text
}

// toolOutputs decodes the kb_search outputs the model received in a request, in order.
func toolOutputs(t *testing.T, request genkitxtest.Request) []ragx.KbSearchOutput {
	t.Helper()
	var outputs []ragx.KbSearchOutput
	for _, m := range request.Body["messages"].([]any) {
		message := m.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		content := toolMessageText(t, message["content"])
		start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
		if start < 0 || end < start {
			continue
		}
		var out ragx.KbSearchOutput
		if json.Unmarshal([]byte(content[start:end+1]), &out) == nil && out.Results != nil {
			outputs = append(outputs, out)
		}
	}
	return outputs
}

// uuidOf makes a stable id from a seed, for models and agents a test names.
func uuidOf(seed string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)) }

func userRequest(text string) model.ChatRequest {
	agentID := testAgentID
	return model.ChatRequest{
		ConversationID: testConversationID, AgentID: &agentID, Trigger: model.TriggerSubmit, UserID: testUserID, Lang: errors.LangVI,
		Message: model.UserMessage{ID: "u-" + uuid.NewString(), Text: text,
			Parts: []byte(fmt.Sprintf(`[{"type":"text","text":%q}]`, text))},
	}
}
