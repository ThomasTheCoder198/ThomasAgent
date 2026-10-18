# Conversation service

## Purpose
Business rules and transactions of the Conversations API (list, get, rename, delete) and the `ChatStore` persistence port of the M2 run loop.

## Entry points
- `ConversationService` / `ConversationServiceImpl` / `NewConversationService` (`conversation_service.go`), plus `ParseLimit` for the controller.
- `ChatStore` / `ChatStoreImpl` / `NewChatStore` (`chat_store.go`): `EnsureConversation`, `LoadHistory`, `AppendUserMessage`, `StartRun`, `DropLastAssistant`, `RecordStep`, `FinishRun`, `SaveSummary`, `ListMessagesRange`, `SetTitle`, `ActiveRun`.
- `encodeCursor` / `decodeCursor` (`cursor.go`).
- `agentservice.KnowledgeBaseLookup` is the only ragx dependency, declared by the agent service and reused here.

## Dependencies
- Uses: `conversation/repository`, `agent/repository`, `agent/service` (the lookup interface), `platform/config` (`ConversationConfig`), `platform/postgres`, named catalog errors, `auth/middleware`
- Used by: `conversation/controller`, `cmd/core`, M2b's `internal/chat`

## Run & test
```bash
cd backend-core
go test ./internal/conversation/service -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatStore_|TestConversationHTTP_|TestTenantIsolation_" -p 1 -count=1
```

## Conventions
- **Paging (A2-11).** The cursor is opaque: base64url of `"<updated_at RFC3339Nano>|<id>"`. Order is `updated_at DESC, id DESC`. `limit` defaults to `CORE_CONVERSATION_PAGE_DEFAULT_LIMIT` (30) and is at most `CORE_CONVERSATION_PAGE_MAX_LIMIT` (100); a bad limit is `limit INVALID`, a bad cursor `cursor INVALID`. The repository reads `limit+1` rows; the extra row means a next page exists. The list never calls ragx, so `lines` is always `["kb"]`.
- **Scope (A2-13, amendment 18).** `Get` asks ragx once for the Agent's KB. Only `KB_NOT_FOUND` sets `scope.kb.missing` (and drops the `kb` tool source); an outage leaves the name empty with `missing` false, keeps the tool source and logs one Warn (`conversation kb lookup failed`). A read never fails on ragx.
- **Ownership (F2a6-3).** Conversation reads, lists and writes require the authenticated end-user in ctx as well as the org; another user receives `CONVERSATION_NOT_FOUND`. Run-ID lookups return `RUN_NOT_ACTIVE` to another user.
- **EnsureConversation (A2-14).** An existing conversation keeps its Agent: a nil `agentID` means "use the stored one", a different one is `agentId INVALID`. A new conversation needs a non-nil agent of this org (`agentId REQUIRED` / `AGENT_NOT_FOUND`). A client id that exists in another org is `CONVERSATION_NOT_FOUND`, so it never leaks.
- **Locking.** Every writer of one conversation (`AppendUserMessage`, `StartRun`, `DropLastAssistant`, `FinishRun`) runs in one transaction that first takes the conversation row `FOR UPDATE`, so `seq`, runs and regenerate never race.
- **Runs (R6).** `StartRun` marks this conversation's overdue `running` rows `failed` (`RUN_DEADLINE_EXCEEDED`) before inserting, so a crashed replica never locks it; a live run is `RUN_ALREADY_ACTIVE`. `ActiveRun` is `RUN_NOT_ACTIVE` for a finished, missing or other-org run.
- **History (A2-15, amendment 10).** `LoadHistory` returns the conversation and its latest `turns × 2` messages with `seq > summary_through_seq`, oldest first, plus `PeakInputTokens`: the largest `input_tokens` of a `model` step in the latest finished (not `running`) run, 0 when none. `FinishRun` inserts the assistant message only when it has parts, and always finishes the run and touches the conversation. `DropLastAssistant` is `NOT_FOUND` unless the id is the last message and an assistant one.
- **Compaction (amendment 16).** `ListMessagesRange(afterSeq, throughSeq, limit)`: `afterSeq` exclusive, `throughSeq` inclusive, oldest first.
