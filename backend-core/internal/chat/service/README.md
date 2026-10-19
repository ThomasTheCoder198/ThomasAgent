# Chat service

## Purpose
The agent run: the loop that answers one user message with the Agent's Knowledge Base tools (R4, R7–R10,
B2-1…B2-14). `RunService.Execute` is the `worker.Executor` of `core worker`, so it runs inside a claimed run, never in
an HTTP handler. It does not claim runs, heartbeat, write the terminal event or relay events to the browser (those are
`chat/worker`, `chat/runstream`); it does not own run rows (`ChatStore`).

## Entry points
- `RunService.Execute(ctx, worker.ExecuteInput)` (`run_service.go`, `run.go`): loads the history, the Agent and the chat
  model, runs the loop, ends the run with the fenced `FinishRun`, and returns the terminal event (`Finish` or `Error`)
  for the handler to write. A lost lease returns `errors.ErrRunLeaseLost` and writes nothing.
- `RunService.Validate(req)` and `RunService.CheckModel(ctx, agentID, requestedModelID)`: the pre-run checks the chat
  endpoint (2b-5) makes before `BeginRun`, so a bad request or an unusable model is a plain JSON error (R5).
- Files: `step.go` (one streamed model call), `tools.go` (the four ragx KB tools: parallel calls, scope scrubbing,
  `<tool_output>` wrapping), `sources.go` (run-wide source numbering and `source-document` events), `rewrite.go`
  (the one corrective rewrite), `history.go` and `prompt.go` (model input).

## Dependencies
- Uses (declared in `deps.go`): `RunStore` (`LoadHistory`, `RecordStep`, `FinishRun` of `ChatStore`), `AgentReader`,
  `ModelResolver`, `ModelRegistry` (`genkitx`), `ToolCaller` (`ragx.Caller.CallTool`), `CallLogger` (`calllog`).
  `chat/worker` (`Executor`, `ExecuteInput`, `CauseCode`), `chat/uistream` (`MessageBuilder`), `chat/model`.
- Used by: `cmd/core` (`core worker` registers it as the `chat.runs` executor; `ServiceContainer.RunService`).

## Run & test
```bash
cd backend-core
go test ./internal/chat/... -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestChatRunLoop_ -p 1 -count=1   # whole worker path
```

## Conventions
- **Events are the only output.** Everything the browser sees goes through `ExecuteInput.Emit`; the stored message is
  built from the same events by `MessageBuilder`. `Emit` never gets `Finish` or `Error`: the terminal event is the
  return value, written after `FinishRun` committed.
- **The run ends once, fenced.** `completed` is never committed after an `Emit` error or after the run context was
  cancelled by a cap or the deadline (ruling F2bW2-6); the run is `failed` with the cap's or the error's code.
- **Stop** arrives through `ExecuteInput.Control`: the loop checks it before each model step and each tool batch, and a
  watcher cancels the executor's own context when a Stop is published, so a model stream or ragx call aborts at once.
  A stopped run saves the parts so far and ends `cancelled`; its terminal event is `Finish` (a Stop is not an error).
- **A lost lease stops the run.** A fenced `RecordStep` or `FinishRun` that returns `ErrRunLeaseLost`, or a handler
  heartbeat that cancelled the context with that cause, makes `Execute` return it and write nothing more.
- **Request id.** The ctx carries the run's request id: `llm.call` and `tool.call` logs and the ragx `X-Request-Id`
  header (and token `rid`) use it. It is never sent to a model provider.
- **Model choice** (B2-8): the run's `requested_model_id`, else the Agent's `chat.default` override, else the
  org/platform role; it must have `chat` and `tools`. The run's `locale` words status labels (B2-9).
- Every tool request gets exactly one tool response; the model never chooses a KB or an org (`ragx.IsScopeKey`).
- A `KB_NOT_FOUND` from either the first search or its corrective retry emits the deletion status and removes KB tools from later model steps. Other corrective retry failures use the first search result.
- A run that streamed nothing and did not complete stores no assistant message, only its row.
- Every `errors.CodeOf` call sits behind an `err != nil` check or goes through `codeOf`.

## Common failures
- `ErrToolSchemaMissing` at startup → `task gen:ragx-client` lost `embedded-spec: true` → restore it in
  `internal/platform/ragx/oapi-codegen.yaml` and regenerate.
- `RUN_STEP_LIMIT` → the model kept calling tools → raise `CORE_CHAT_MAX_STEPS` or look at the tool outputs.
- A run ends `RUN_OUTPUT_TOO_LARGE` or `RUN_DEADLINE_EXCEEDED` with a partial answer saved → a stream cap or the run
  deadline cancelled the run context → `CORE_CHAT_RUN_STREAM_MAX_*`, `CORE_CHAT_RUN_DEADLINE`.
- `LLM_MODEL_CAPABILITY_MISMATCH` on a run that passed `CheckModel` → the model lost a capability between `BeginRun` and
  execution → the run ends with that catalog error; fix the model's capabilities in Settings.
