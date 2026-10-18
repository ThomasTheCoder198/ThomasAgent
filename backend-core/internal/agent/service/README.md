# Agent service

## Purpose
Business rules and transactions of the Agents API: create, read, patch and delete an Agent, with its KB and model-override checks and audit.

## Entry points
- `AgentService` / `AgentServiceImpl` / `NewAgentService` (`agent_service.go`)
- `KnowledgeBaseLookup`: the part of `ragx.Caller` the service needs (`GetKnowledgeBase`); `ModelResolver`: the part of `llmprovider`'s `ResolveService` it needs (`ResolveModel`). Both are declared here, by the consumer.
- Audit action names (`audit_events.go`)

## Dependencies
- Uses: `agent/repository`, `platform/ragx`, `llmprovider/model` and its `ResolveService`, `platform/audit`, `platform/postgres`, named catalog errors
- Used by: `agent/controller`, `cmd/core`

## Run & test
```bash
cd backend-core
go build ./internal/agent/service
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestAgentHTTP_ -p 1 -count=1
```

## Conventions
- **KB (ruling A2-9, amendment 18).** Create and a patch that carries `kbId` ask ragx for the KB under the org's token; `KB_NOT_FOUND` becomes `AGENT_KB_INVALID` (422), any other ragx failure passes through (`RAGX_UNAVAILABLE`, 503). A nil `kbId` in a patch answers `kbId REQUIRED` before ragx is called. Reads never fail on ragx: only `KB_NOT_FOUND` sets `kbMissing`; an outage leaves `kbName` empty with `kbMissing` false and logs one Warn (`agent kb lookup failed`). A list asks ragx once per distinct KB.
- **Overrides (ruling A2-10).** Only `chat.default`, `chat.fast` and `vision` can be pinned. Each pinned model must resolve and fit the role (`FitsRole`); otherwise `modelOverrides.<role>` is `INVALID`. Roles are checked in sorted order so the first error is stable. `null` clears a role. The KB and model checks call outside services, so they run before the row lock.
- **Audit.** `agent.create`, `agent.update` and `agent.delete` (actor `owner`, tenant from ctx) are written in the same transaction as the write; a refused patch writes nothing. The rows carry no metadata.
- **Delete cascade.** Deleting an agent also removes, through the `ON DELETE CASCADE` foreign keys of the chat schema, every conversation, message and run of that agent, including an active run. The audit row does not count them.
