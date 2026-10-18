# Conversation

## Purpose

Owns conversations, their messages (AI SDK `UIMessage` parts), runs and run steps, and the org-scoped Conversations API (`/api/v1/orgs/{orgId}/conversations`). A conversation belongs to one authenticated end-user and is bound to one Agent at creation. It does not run models; the M2 chat runtime drives it through the `ChatStore` persistence port. The API reads one ragx fact: the name of the Agent's KB.

## Entry points

The domain follows the layers of every core domain; a request flows `controller → service → repository`, and `model` and `dto` are plain types.

| Folder | Holds | Example |
|---|---|---|
| `controller/` | HTTP handlers only: read the request, call one service, write a DTO | `ConversationController.HandleListConversations` |
| `dto/` | JSON request and response types and their conversions from `model` types | `ConversationSummaryResponse`, `ConversationDetailResponse`, `ChatScopeResponse` |
| `service/` | Business rules and transactions: paging, scope, the chat store | `ConversationService`, `ChatStore` / `ChatStoreImpl` |
| `model/` | Domain types and their rules | `Conversation`, `Message`, `History`, `Run`, `RunStep`, `Page`, `PageMeta` |
| `repository/` | SQL only, one table per file (`XRepo` interface + implementation) | `ConversationRepo`, `MessageRepo`, `RunRepo` |

- Routes (registered in `cmd/core/routes.go` inside `RequireOrg`): `GET /api/v1/orgs/{orgId}/conversations`, and `GET`, `PATCH` and `DELETE /api/v1/orgs/{orgId}/conversations/{conversationId}`. UI wired in 2c-x.
- `ChatStore` is the port M2b's run loop uses; `cmd/core/container.go` exposes it as `ServiceContainer.ChatStore`.

## Dependencies

- Uses: `agent/repository` (agent lookups), `agent/service` (`KnowledgeBaseLookup`, implemented by `platform/ragx`), `platform/postgres` (DBTX, `WithTransaction`), `platform/tenant`, `platform/httpx`, named catalog errors.
- Used by: `cmd/core`, the M2 chat runtime.

## Run & test

```bash
cd backend-core
go test ./internal/conversation/... -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration|TestChatStore_|TestConversationHTTP_|TestTenantIsolation_" -p 1 -count=1
```

## Conventions

- Every row carries `tenant_id` and is read under the tenant in ctx, never under a client parameter. Conversation access also checks `user_id` from the authenticated context; another user sees the same not-found result as another org.
- Messages are keyed `(conversation_id, id)`, so a client-chosen id is scoped to its conversation.
- `seq` is assigned by `InsertMessage` while the caller holds the conversation row lock (`GetConversationForUpdate`).
- One `running` run per conversation, enforced by the partial unique index `uq_runs_one_active`; a violation is `RUN_ALREADY_ACTIVE`.
- A child insert proves its parent is in the tenant (`INSERT ... SELECT ... FROM <parent> WHERE tenant_id = ...`), and composite foreign keys `(parent_id, tenant_id)` back it in the database, so no row can name another org's parent.
- `GetRun` answers `RUN_NOT_ACTIVE` for another org's run, so a run id never leaks.
- Paging (A2-11), the chat store rules (A2-14, A2-15, `History.PeakInputTokens`) and the scope rules are in `service/README.md`.
