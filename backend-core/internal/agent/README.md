# Agent

## Purpose

Agents are an org's assistants: a name, instructions, optional per-role model overrides and exactly one ragx Knowledge Base. The domain owns the `agents` table and the org-scoped Agents API (`/api/v1/orgs/{orgId}/agents`). Every operation requires the org from trusted context; public input never chooses a tenant.

## Entry points

The domain follows the same layers as every core domain; a request flows `controller → service → repository`, and `model` and `dto` are plain types.

| Folder | Holds | Example |
|---|---|---|
| `controller/` | HTTP handlers only: read the request, call one service, write a DTO | `AgentController.HandleCreateAgent` |
| `dto/` | JSON request and response types and their conversions to `model` types | `CreateAgentRequest`, `UpdateAgentRequest`, `AgentResponse` |
| `service/` | Business rules and transactions: KB and model checks, audit | `AgentService`, `KnowledgeBaseLookup`, `ModelResolver` |
| `repository/` | SQL only (`XRepo` interface + implementation) | `AgentRepo` / `NewAgentRepo` |
| `model/` | Domain types and their rules | `Agent`, `AgentInput`, `AgentPatch`, `Agent.Apply`, `IsOverridable` |

`cmd/core/container.go` builds the repository, service and controller; `cmd/core/routes.go` registers the five routes inside the `RequireOrg` group.

## Dependencies

- Uses: `platform/ragx` (KB lookup), `llmprovider` (`ResolveModel`, `FitsRole`), `platform/audit`, `platform/postgres` (DBTX, `WithTransaction`), `platform/tenant`, `platform/httpx`, named catalog errors.
- Used by: `cmd/core`, `internal/conversation` (conversations reference an agent) and the M2 chat runtime.

## Run & test

```bash
cd backend-core
go test ./internal/agent/... -count=1 -p 1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestAgentHTTP_ -p 1 -count=1
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration" -p 1 -count=1
```

## Conventions

- Every row carries `tenant_id` and is read under the tenant in ctx (the org, set by `RequireOrg`), never under a client parameter.
- The agent name is unique per org; a duplicate becomes `CONFLICT` on `name`.
- `kb_id` has no foreign key: the KB lives in ragx and is validated there under the org's own token. Rules for the KB, overrides and audit are in `service/README.md`.
- `ListAgents` returns oldest first, so the first agent is the chat default.
