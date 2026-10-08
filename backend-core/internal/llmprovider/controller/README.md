# LLM provider controller

## Purpose
HTTP handlers only: read the request, call one service, write a DTO.

## Entry points
- `ProviderController` (`provider_controller.go`)
- `LLMModelController` (`llm_model_controller.go`)
- `RoleController` (`role_controller.go`)
- `ResolveController` (`resolve_controller.go`, internal route)

## Dependencies
- Uses: `llmprovider/service`, `llmprovider/dto`, `platform/httpx`
- Used by: `cmd/core`

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/controller
```

## Conventions
Covered over HTTP by `tests/integration/llmprovider_http_test.go`.
