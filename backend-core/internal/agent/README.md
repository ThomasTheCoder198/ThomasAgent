# Agent

## Purpose

Agents are an org's assistants: a name, instructions, optional per-role model overrides and exactly one ragx Knowledge Base. The domain owns the `agents` table and (from 2a-5) the Agents API. It does not talk to ragx itself except to check a KB through `ragx.Caller.GetKnowledgeBase`.

## Entry points

The domain follows the layers of every core domain; a request flows `controller → service → repository`, and `model` and `dto` are plain types. Only `model/` and `repository/` exist so far (Task 2a-1); the rest arrives with the Agents API.

| Folder | Holds | Example |
|---|---|---|
| `model/` | Domain types and their rules | `Agent`, `AgentInput`, `AgentPatch`, `IsOverridable` |
| `repository/` | SQL only (`XRepo` interface + implementation) | `AgentRepo` / `NewAgentRepo` |

## Dependencies

- Uses: `platform/postgres` (DBTX), `platform/tenant`, named catalog errors, `llmprovider/model` for roles.
- Used by: `internal/conversation` (conversations reference an agent) and the M2 chat runtime.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration" -p 1 -count=1
```

## Conventions

- Every row carries `tenant_id` and is read under the tenant in ctx (the org, set by `RequireOrg`), never under a client parameter.
- The agent name is unique per org; a duplicate becomes `CONFLICT` on `name`.
- `kb_id` has no foreign key: the KB lives in ragx.
- `ListAgents` returns oldest first, so the first agent is the chat default.
