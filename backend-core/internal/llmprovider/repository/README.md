# LLM provider repository

## Purpose
SQL for providers, models and role assignments. Pure data access: every method takes the pool or the service transaction.

## Entry points
- `ProviderRepo` (`provider_repository.go`)
- `LLMModelRepo` (`llm_model_repository.go`)
- `RoleRepo` (`role_repository.go`)

## Dependencies
- Uses: `llmprovider/model`, `platform/postgres`, `platform/tenant`
- Used by: `llmprovider/service`

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/repository
```

## Conventions
Every method scopes rows by the tenant in ctx; covered by the service tests against real Postgres.
