# App proxy service

## Purpose
Checks what the browser asked for and forwards it to the app with a scope taken from the method.

## Entry points
- `AppProxyService.Forward` (`app_proxy_service.go`); `Forwarder` is the one outside call, satisfied by `*ragx.Caller`.

## Dependencies
- Uses: `appproxy/model`, `platform/ragx`, `platform/httpx` (`ValidIdempotencyKey`), named catalog errors.
- Used by: `appproxy/controller`, `cmd/core`.

## Run & test
```bash
cd backend-core
go test ./internal/appproxy/service -count=1
```

## Conventions
- Path rules: `^[A-Za-z0-9._~/-]+$`, no `..`, no `.` segment (JoinPath would rewrite it), no `//` (checked on the raw path, so a leading `//` is refused), not empty, no trailing `/`; otherwise `NOT_FOUND` before any ragx call.
- Unknown method → `METHOD_NOT_ALLOWED`; bad `Idempotency-Key` → `VALIDATION_FAILED` field `Idempotency-Key`.
- The `oid` comes from the tenant in ctx (set by `RequireOrg`), never from the request.
