# LLM provider model

## Purpose
Domain types of providers, models, capabilities and roles, plus the rules that belong to them. No I/O.

## Entry points
- `Provider`, `ProviderInput`, `StoredProvider` (`provider.go`)
- `LLMModel`, `LLMModelInput`, `FitsRole` (`llm_model.go`)
- `Capability` (`capability.go`); `Role`, `RequirementsForRole`, `ResolvedModel` (`role.go`)
- `RemoteModel` (`remote_model.go`), `SyncResult` (`sync_result.go`)

## Dependencies
- Uses: `github.com/google/uuid`
- Used by: every other llmprovider layer

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/model
```

## Conventions
`StoredProvider` carries the vault reference and never leaves the service layer.
