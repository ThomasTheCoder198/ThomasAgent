# Conversation

## Purpose

Owns conversations, their messages (AI SDK `UIMessage` parts), runs and run steps. A conversation is bound to one Agent at creation. It does not run models or call ragx; the M2 chat runtime drives it through a persistence port (from 2a-6).

## Entry points

The domain follows the layers of every core domain; a request flows `controller → service → repository`, and `model` and `dto` are plain types. Only `model/` and `repository/` exist so far (Task 2a-1).

| Folder | Holds | Example |
|---|---|---|
| `model/` | Domain types and their rules | `Conversation`, `Message`, `History`, `Run`, `RunStep` |
| `repository/` | SQL only, one table per file (`XRepo` interface + implementation) | `ConversationRepo`, `MessageRepo`, `RunRepo` |

## Dependencies

- Uses: `platform/postgres` (DBTX), `platform/tenant`, named catalog errors.
- Used by: the M2 chat runtime and the Conversations API.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration" -p 1 -count=1
```

## Conventions

- Every row carries `tenant_id` and is read under the tenant in ctx, never under a client parameter.
- Messages are keyed `(conversation_id, id)`, so a client-chosen id is scoped to its conversation.
- `seq` is assigned by `InsertMessage` while the caller holds the conversation row lock (`GetConversationForUpdate`).
- One `running` run per conversation, enforced by the partial unique index `uq_runs_one_active`; a violation is `RUN_ALREADY_ACTIVE`.
- A child insert proves its parent is in the tenant (`INSERT ... SELECT ... FROM <parent> WHERE tenant_id = ...`), and composite foreign keys `(parent_id, tenant_id)` back it in the database, so no row can name another org's parent.
- `GetRun` answers `RUN_NOT_ACTIVE` for another org's run, so a run id never leaks.
