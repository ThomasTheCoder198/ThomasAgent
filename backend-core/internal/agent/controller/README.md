# Agent controller

## Purpose
HTTP handlers only: read the request, call one service, write a DTO.

## Entry points
- `AgentController` / `NewAgentController` (`agent_controller.go`): `HandleListAgents`, `HandleCreateAgent`, `HandleGetAgent`, `HandleUpdateAgent`, `HandleDeleteAgent` for `/api/v1/orgs/{orgId}/agents[/{agentId}]`, mounted inside `RequireOrg`.

## Dependencies
- Uses: `agent/service`, `agent/dto`, `platform/httpx`, named catalog errors
- Used by: `cmd/core`

## Run & test
```bash
cd backend-core
go build ./internal/agent/controller
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestAgentHTTP_ -p 1 -count=1
```

## Conventions
A malformed agent id answers `AGENT_NOT_FOUND`, like a missing one. Bodies are decoded with `httpx.DecodeJSON` (unknown fields refused, size capped). Covered over HTTP by `tests/integration/agent_http_test.go`.
