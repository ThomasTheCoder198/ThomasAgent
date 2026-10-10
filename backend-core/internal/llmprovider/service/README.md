# LLM provider service

## Purpose
Business rules and transactions: provider CRUD and key rotation, manual models, catalog sync, role assignment and resolution.

## Entry points
- `ProviderService` (`provider_service.go`)
- `LLMModelService` (`llm_model_service.go`)
- `ModelSyncService` (`model_sync_service.go`)
- `RoleService` (`role_service.go`)
- `ResolveService` (`resolve_service.go`): role → model with its decrypted key, for the M2 agent runtime; no HTTP route
- Audit action names (`audit_events.go`)

## Dependencies
- Uses: `llmprovider/repository`, `llmprovider/client`, `platform/vault`, `platform/audit`, `platform/postgres`
- Used by: `llmprovider/controller`, `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/llmprovider/service -count=1
```

## Conventions
Each file holds one `XService` interface, its `XServiceImpl` and `NewXService`. Mutations and their audit rows share `postgres.WithTransaction`.
