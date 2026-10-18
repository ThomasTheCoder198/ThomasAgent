# Agent model

## Purpose
Domain types of agents and the field limits that mirror the `agents` table CHECK constraints. No I/O.

## Entry points
- `Agent`, `AgentInput`, `AgentPatch` (`agent.go`)
- `OverridableRoles`, `IsOverridable`: the roles an agent may pin to its own model (`chat.default`, `chat.fast`, `vision`)

## Dependencies
- Uses: `llmprovider/model` (roles), `github.com/google/uuid`
- Used by: `agent/repository` and the later agent layers

## Run & test
```bash
cd backend-core
go build ./internal/agent/...
```

## Conventions
In `AgentPatch` a nil field is unchanged; in `ModelOverrides` a nil value removes that role's override. The `*MaxRunes` limits count characters, as Postgres `char_length` does.
