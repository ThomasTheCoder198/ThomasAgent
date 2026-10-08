# Auth controller

## Purpose

Translate login, logout and current-user HTTP requests into auth service calls, cookies and response DTOs.

## Entry points

- `NewAuthController(AuthControllerParams)` builds the controller.
- `HandleLogin`, `HandleLogout`, and `HandleMe` are registered in `cmd/core/routes.go`.

## Dependencies

- Uses: auth service, limiter, DTOs, middleware constants and shared `httpx` responses.
- Used by: `cmd/core/server.go` and `cmd/core/routes.go`.

## Run & test

```bash
cd backend-core
go test ./internal/auth/... -count=1
```

## Conventions

Login derives platform tenant in request context and uses trusted-proxy settings only when injected. Credentials and limiter secrets never enter logs.

## Common failures

An untrusted forwarding header cannot choose the client IP; CSRF failures use the shared error response.
