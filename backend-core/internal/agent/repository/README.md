# Agent repository

## Purpose
SQL for the `agents` table. Every method takes the pool or a service transaction and scopes rows by the tenant in ctx, which is the org under `RequireOrg`.

## Entry points
- `AgentRepo` / `NewAgentRepo` (`agent_repository.go`)
- `GetAgentForUpdate` locks the row for a read-modify-write PATCH.

## Dependencies
- Uses: `agent/model`, `platform/postgres`, `platform/tenant`, `llmprovider/model`
- Used by: the agent service (2a-5)

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestChatRepositories_RoundTripAndTenantScope -count=1
```

## Conventions
A duplicate name in one org becomes `CONFLICT` with the field `name`; a missing or foreign agent is `AGENT_NOT_FOUND`. Model overrides are stored as jsonb `{role: modelUuid}` and decoded to a non-nil map.
