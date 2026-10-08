# LLM provider DTO

## Purpose
JSON request and response bodies of the provider, model, role and resolve endpoints and their conversions.

## Entry points
- `provider.go`, `llm_model.go`, `role.go`, `resolved_model.go`
- `DeleteResponse` (`common.go`)

## Dependencies
- Uses: `llmprovider/model`
- Used by: `llmprovider/controller`

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/dto
```

## Conventions
Request types are write-only for API keys; only `ResolvedModelResponse` (internal route) contains a key.
