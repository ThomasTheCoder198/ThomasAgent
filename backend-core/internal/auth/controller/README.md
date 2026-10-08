# Auth controller

## Purpose
HTTP handlers for login, logout and the current user. Reads the request, calls `AuthService` and the limiter, writes DTOs and cookies; no business rules.

## Entry points
- `AuthController`, `NewAuthController` (`auth_controller.go`)
- `clientIP`: the address the limiter counts, honoring a trusted proxy header (`client_ip.go`)

## Dependencies
- Uses: `auth/service`, `auth/limiter`, `auth/dto`, `auth/util`, `auth/middleware`, `platform/httpx`
- Used by: `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth/controller -count=1
```

## Conventions
A forwarding header is honored only when the direct peer is a trusted proxy; the rightmost untrusted hop is the client.
