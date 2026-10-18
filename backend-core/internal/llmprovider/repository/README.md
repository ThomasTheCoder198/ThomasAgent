# LLM provider repository

## Purpose
SQL for providers, models and role assignments. Pure data access: every method takes the pool or the service transaction.

## Entry points
- `ProviderRepo` (`provider_repository.go`)
- `LLMModelRepo` (`llm_model_repository.go`): the sync upsert keeps capabilities when `capabilities_edited`; `UpdateCapabilities` is the operator's edit; `GetModelForShare` (role assignment) and `GetModelForUpdate` (capability edit) take conflicting row locks so the two serialize
- `RoleRepo` (`role_repository.go`): `RolesUsingModel` lists the roles any tenant pinned to a model

## Dependencies
- Uses: `llmprovider/model`, `platform/postgres`, `platform/tenant`
- Used by: `llmprovider/service`

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/repository
```

## Conventions
Every method scopes rows by the tenant in ctx; covered by the service tests against real Postgres. `RoleRepo` also has `…In` methods that take the tenant explicitly: resolution under an org reads the org's rows and the platform's in one request.
