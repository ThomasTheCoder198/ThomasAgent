# Conversation repository

## Purpose
SQL for the `conversations`, `messages`, `runs` and `run_steps` tables. Every method takes the pool or a service transaction and scopes conversation rows by the tenant and authenticated user in ctx. Conversation insertion derives its owner from ctx and refuses a conflicting supplied owner.

## Entry points
- `ConversationRepo` (`conversation_repository.go`): `ListPage` returns up to `limit+1` rows so the caller can tell a next page exists.
- `MessageRepo` (`message_repository.go`): `ListLatest`, `ListAll` and `ListRange` return oldest first.
- `RunRepo` (`run_repository.go`): `ExpireStale` fails this conversation's overdue `running` rows (R6); `PeakInputTokens` is the largest `input_tokens` of a `model` step in the conversation's latest finished run (0 when none), the input of M2b's compaction trigger.

## Dependencies
- Uses: `conversation/model`, `platform/postgres`, `platform/tenant`, `auth/middleware`
- Used by: the conversation service and chat store

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration" -p 1 -count=1
```

## Conventions
Every insert selects its parent row under the tenant in ctx, so a child can never attach to another org's parent: `InsertConversation` answers `AGENT_NOT_FOUND` for a foreign agent, `InsertMessage` and `InsertRun` answer `CONVERSATION_NOT_FOUND` for a foreign conversation, and `InsertStep` answers `RUN_NOT_ACTIVE` for a foreign run. The composite foreign keys `(parent_id, tenant_id)` of the migration make the database refuse the same writes. A missing or foreign conversation is `CONVERSATION_NOT_FOUND`; a missing or foreign run is `RUN_NOT_ACTIVE`. A repeated message id in one conversation is `CONFLICT` on `id`. A second `running` run is `RUN_ALREADY_ACTIVE`, recognised by the constraint name `uq_runs_one_active`.
