# App proxy model

## Purpose
Request and response types of the app proxy, and the method-to-scope table.

## Entry points
- `AppCall`: what the controller read from the browser; `AppResponse`: what goes back.
- `ForwardedMethods()` and `ScopeFor(method)`: both read `methodScopes`, so the registered methods and the token scope cannot drift.

## Dependencies
- Uses: `platform/apptoken` (scopes), `platform/httpx` (header name).
- Used by: `appproxy/service`, `appproxy/controller`, `cmd/core`.

## Run & test
```bash
cd backend-core
go test ./internal/appproxy/... -count=1
```

## Conventions
Add a method here only with its scope; `OPTIONS`, `TRACE` and `CONNECT` stay out on purpose.
