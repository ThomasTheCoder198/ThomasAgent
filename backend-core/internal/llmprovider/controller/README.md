# LLM provider controllers

## Purpose

Translate public provider, model and role requests and the internal resolve request into service calls and shared response DTOs.

## Entry points

- `NewProviderController`, `NewModelController`, `NewModelRoleController`, `NewInternalModelController` construct one controller per API surface.
- Exported `HandleXxx(*gin.Context)` methods are registered only in `cmd/core/routes.go`.

## Dependencies

- Uses: `llmprovider/service`, `llmprovider/dto`, `llmprovider/model`, `httpx`.
- Used by: `cmd/core/server.go` and `cmd/core/routes.go`.

## Run & test

```bash
cd backend-core
go test ./internal/llmprovider/... -count=1
```

## Conventions

Controllers do not register routes or authenticate requests. Public responses never contain provider keys. The internal resolve route is protected by `llmprovider/middleware.RequireServiceToken`.

## Common failures

Invalid path UUIDs and JSON bodies return catalog validation errors through `httpx.Fail`.
