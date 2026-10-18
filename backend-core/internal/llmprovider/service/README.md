# LLM provider service

## Purpose
Business rules and transactions: provider CRUD and key rotation, manual models, catalog sync, role assignment (platform and per-org) and resolution.

## Entry points
- `ProviderService` (`provider_service.go`)
- `LLMModelService` (`llm_model_service.go`): manual models and `UpdateCapabilities` (capability edit, audited `model.update`, refuses edits that break an assigned role; it locks the model row `FOR UPDATE` before reading the roles, because role assignment only holds `FOR SHARE` and share locks do not conflict with each other)
- `ModelSyncService` (`model_sync_service.go`): OpenRouter and Google; `forCore` trims a Google list to chat models and counts the rest as skipped (ruling F2a3-1)
- `RoleService` (`role_service.go`)
- `OrgRoleService` (`org_role_service.go`): list, assign and clear the org's chat-role overrides; audited as `model_role.assign_org` / `model_role.clear_org`
- `ResolveService` (`resolve_service.go`): role (org → platform → decision→chat.fast) or Agent-pinned model → model with its decrypted key and `KeyVersion`, for the M2 agent runtime; no HTTP route. `RoleService.ClearRole` clears a platform role (idempotent, audited)
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
