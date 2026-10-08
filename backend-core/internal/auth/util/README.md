# Auth util

## Purpose
Session cookie names and the helpers that set and clear them, shared by the controller and the session middleware.

## Entry points
- `SessionCookie`, `CSRFCookie`, `CSRFHeader`
- `SetSessionCookies`, `ClearSessionCookies` (`session_cookie.go`)

## Dependencies
- Uses: `auth/model`
- Used by: `auth/controller`, `auth/middleware`

## Run & test
```bash
cd backend-core
go build ./internal/auth/util
```

## Conventions
The session cookie is HttpOnly; the CSRF cookie is readable so the frontend can echo it. Both use Path `/`, SameSite Lax and the configured Secure flag.
