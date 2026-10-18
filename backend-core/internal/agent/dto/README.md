# Agent dto

## Purpose
All JSON request and response types of the Agents API, and their conversion to and from `agent/model` types.

## Entry points
- `CreateAgentRequest.ToInput`, `UpdateAgentRequest.ToPatch` (`agent.go`): requests to `model.AgentInput` / `model.AgentPatch`; in an update, absent fields are unchanged and a `null` `modelOverrides` value removes that role's override.
- `AgentResponse`, `NewAgentResponse`, `NewAgentResponses` (`agent.go`): an empty list is `[]`, never `null`.

## Dependencies
- Uses: `agent/model`, `llmprovider/model` (roles), `github.com/google/uuid`
- Used by: `agent/controller`

## Run & test
```bash
cd backend-core
go build ./internal/agent/dto
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestAgentHTTP_ -p 1 -count=1
```

## Conventions
DTOs carry no tenant id. Services take `model` types only, so the controller never passes a DTO down.
