# LLM provider DTO

## Purpose
JSON request and response bodies of the provider, model and role endpoints and their conversions.

## Entry points
- `provider.go`, `llm_model.go` (`LLMModelResponse.CapabilitiesEdited`, `UpdateModelRequest`), `role.go`, `org_role.go` (`OrgRoleResponse`)
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
Request types are write-only for API keys; no response contains a key.
