# Conversation service

## Purpose
Business rules and transactions of the Conversations API (list, get, rename, delete) and the `ChatStore` persistence port of the M2 run loop.

## Entry points
- `ConversationService` / `ConversationServiceImpl` / `NewConversationService` (`conversation_service.go`), plus `ParseLimit` for the controller.
- `RetentionService` / `RetentionServiceImpl` / `NewRetentionService` (`retention_service.go`) scans all tenants in bounded batches from an injected clock; the scheduled core job is its only caller.
- `ChatStore` / `ChatStoreImpl` / `NewChatStore` (`chat_store.go`). API side: `EnsureConversation`, `LoadHistory`, `BeginRun`, `RunStatus`, `ActiveRun`. Worker side, fenced by the run lease: `ClaimRun`, `RenewLease`, `RecordStep`, `FinishRun`, `FailRun`. Also `SaveSummary`, `ListMessagesRange`, `SetTitle`.
- `ConversationService.Get` fills `lastRun` (the latest run of the conversation: id, status, error code, request id; nil when none) after the tenant and user check.
- `encodeCursor` / `decodeCursor` (`cursor.go`).
- `agentservice.KnowledgeBaseLookup` is the only ragx dependency, declared by the agent service and reused here.

## Dependencies
- Uses: `conversation/repository`, `agent/repository`, `agent/service` (the lookup interface), `platform/config` (`ConversationConfig`), `platform/postgres`, named catalog errors, `auth/middleware`
- Used by: `conversation/controller`, `cmd/core`, M2b's `internal/chat`

## Run & test
```bash
cd backend-core
go test ./internal/conversation/service -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatStore_|TestRunLease_|TestConversationHTTP_|TestTenantIsolation_" -p 1 -count=1
```

## Conventions
- **Paging (A2-11).** The cursor is opaque: base64url of `"<updated_at RFC3339Nano>|<id>"`. Order is `updated_at DESC, id DESC`. `limit` defaults to `CORE_CONVERSATION_PAGE_DEFAULT_LIMIT` (30) and is at most `CORE_CONVERSATION_PAGE_MAX_LIMIT` (100); a bad limit is `limit INVALID`, a bad cursor `cursor INVALID`. The repository reads `limit+1` rows; the extra row means a next page exists. The list never calls ragx, so `lines` is always `["kb"]`.
- **Scope (A2-13, amendment 18).** `Get` asks ragx once for the Agent's KB. Only `KB_NOT_FOUND` sets `scope.kb.missing` (and drops the `kb` tool source); an outage leaves the name empty with `missing` false, keeps the tool source and logs one Warn (`conversation kb lookup failed`). A read never fails on ragx.
- **Ownership (F2a6-3).** Conversation reads, lists and writes require the authenticated end-user in ctx as well as the org; another user receives `CONVERSATION_NOT_FOUND`. Run-ID lookups return `RUN_NOT_ACTIVE` to another user.
- **EnsureConversation (A2-14).** An existing conversation keeps its Agent: a nil `agentID` means "use the stored one", a different one is `agentId INVALID`. A new conversation needs a non-nil agent of this org (`agentId REQUIRED` / `AGENT_NOT_FOUND`). A client id that exists in another org is `CONVERSATION_NOT_FOUND`, so it never leaks.
- **Locking.** Every writer of one conversation (`BeginRun`, `FinishRun`) runs in one transaction that first takes the conversation row `FOR UPDATE`, so `seq`, runs and regenerate never race. The lock order is always conversation, then run.
- **BeginRun (amendment W3).** The only way to start a run, so no run exists without its job. One transaction under the conversation lock: expire this conversation's stale runs after `CORE_CHAT_FINISH_TIMEOUT` or an expired lease (R6); apply the message mutation; insert the run (a live run is `RUN_ALREADY_ACTIVE`); enqueue the `chat.runs` outbox row. A live executor keeps its lease through the finish window to save a partial answer; an expired lease is `RUN_INTERRUPTED`, and an unleased run beyond the window is `RUN_DEADLINE_EXCEEDED`. A `UserMessage` is inserted and touches the conversation. Regenerate takes the last **assistant** id (that message is dropped, its run row stays) or the last **user** id with no answer after it (nothing dropped, W7); any other id is `NOT_FOUND`. Neither a message nor a regenerate id, or no assistant message id, is `VALIDATION_FAILED`; an agent id that differs from the conversation's is too. Tenant, user and request id come from ctx; a ctx without a valid request id gets one `job_` id, so the run row and the outbox row always carry the same one.
- **The lease (amendment W5).** `ClaimRun` takes `lease_token` (fresh per attempt), `lease_owner` and `lease_until` for a `running`, unclaimed run before its deadline, and explains a refusal with one read: `NotRunning`, `QueuedPastDeadline`, `Expired` (lease lapsed: the executor is gone) or `HeldElsewhere`. `RenewLease`, `RecordStep` and `FinishRun` are fenced by `lease_token = $t AND status = 'running' AND lease_until > now()` in their own statement (times are the database clock); no row is `errors.ErrRunLeaseLost` and nothing is written. The fenced statement takes the run row lock, so it serializes with `FailRun`; one side wins, never both. `FinishRun` and `FailRun` clear the lease. `FailRun(onlyIfExpired)` is the interrupted and queued-past-deadline transition and reports whether it changed the row.
- **Runs (R6).** `ActiveRun` is `RUN_NOT_ACTIVE` for a finished, missing or other-org run. `RunStatus` is `NOT_FOUND` for a run the user cannot see (missing, other org or other user).
- **History (A2-15, amendment 10).** `LoadHistory` returns the conversation and its latest `turns × 2` messages with `seq > summary_through_seq`, oldest first, plus `PeakInputTokens`: the largest `input_tokens` of a `model` step in the latest finished (not `running`) run, 0 when none. `FinishRun` inserts the assistant message only when it has parts, always finishes the run and touches the conversation.
- **Compaction (amendment 16).** `ListMessagesRange(afterSeq, throughSeq, limit)`: `afterSeq` exclusive, `throughSeq` inclusive, oldest first.
