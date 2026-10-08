# LLM provider middleware

## Purpose

Authenticate the internal model resolution route with the configured service token.

## Entry points

- `RequireServiceToken(token)` returns a Gin middleware function.

## Dependencies

- Uses: `httpx`, `internal/errors` and trusted platform tenant context.
- Used by: `cmd/core/routes.go`.

## Run & test

```bash
cd backend-core
go test ./internal/llmprovider/... -count=1
```

## Conventions

SHA-256 token digests are compared in constant time. Successful authentication sets the platform tenant in `c.Request.Context()` and prevents response caching.

## Common failures

Missing or invalid bearer tokens return `INTERNAL_TOKEN_INVALID`.
