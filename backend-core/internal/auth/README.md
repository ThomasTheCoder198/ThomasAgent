# Auth

## Purpose

Bootstraps the operator, verifies Argon2id passwords, issues hashed sessions, and serves cookie-based login, logout and current-user endpoints.
M0.2 data belongs to the [Platform tenant](../../../docs/glossary.md) (`default`), using the migration's platform defaults. M1 business tenancy must take `tenant_id` from trusted context, never client or model parameters.

## Entry points

- `NewRepository`, `NewService`, `EnsureOwner`, `Login`, `Authenticate`, and `Logout`.
- `NewHandler(...).Mount` registers POST `/api/v1/auth/login`, POST `/api/v1/auth/logout`, and GET `/api/v1/auth/me`.
- `RequireSession` authenticates cookies, checks CSRF on unsafe methods and exposes `UserFrom` to protected handlers; `NewLimiter` counts login attempts.

## Dependencies

- Uses: Postgres, Redis, typed `config.AuthConfig`, Argon2id, UUIDs, catalog errors, audit recording, chi and shared httpx boundaries.
- Used by: core domain services and HTTP routing; startup mounting is a later task.

## Run & test

```bash
cd backend-core
go test ./internal/auth/... -count=1 -p 2
```

Tests use `pgtest.Start`, migrated Docker Postgres, Redis and Ryuk. Real HTTP stream gates exercise tracing, access logging, session cookies and CSRF checks, three incremental frames, disconnect cancellation and committed-panic recovery without JSON corruption.

## Conventions

Passwords, raw session tokens and secrets must never enter logs, errors or audit metadata. Errors propagate to the boundary for one log and shared HTTP formatting.

`thomas_session` is HttpOnly; `thomas_csrf` is readable by the browser. Both use Path `/`, SameSite Lax and the constructor's Secure flag. Their absolute expiry matches the session. Unsafe methods require `X-CSRF-Token` equal to the authenticated session's token with constant-time comparison; GET, HEAD and OPTIONS are safe. Logout requires CSRF, deletes the session and clears both cookies.

The Redis limiter uses `thomas:auth:login:` plus client IP from RemoteAddr and counts every attempt, including successful login. Its maximum attempts and fixed window come from constructor configuration. Redis failures reject the request with INTERNAL_ERROR. If assigning expiry fails, cleanup removes the incremented key to avoid an immortal rate limit; any cleanup failure is retained for boundary logging. Middleware preserves the original response writer and request cancellation; streaming has no duration limit. The shared router supplies tracing, access logs, error logging once and committed-response recovery.

## Common failures

- An empty owner email or short bootstrap password requires CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD. Expired or missing sessions return catalog auth errors.
- AUTH_CSRF_INVALID means the unsafe request omitted or mismatched X-CSRF-Token; RATE_LIMITED means the configured client-IP attempt limit was exceeded. Login errors use the same response for unknown email and wrong password; `/me` exposes only ID, email and display name.

Emails are normalized before lookup. Only SHA-256 token hashes are stored; CSRF tokens accompany issued sessions. Session lifetime and bootstrap password length come from typed config. Bootstrap is idempotent when a user already exists. Constructor hash errors are retained and returned by owner bootstrap and login. Login returns an INTERNAL_ERROR when audit recording fails and attempts to delete the newly created session. Both audit and cleanup errors are retained for boundary logging; the raw token is never returned on failure. If cleanup also fails, an undisclosed session row can remain until expiry. Expired authentication returns AUTH_SESSION_EXPIRED after successful cleanup. Failed expiry cleanup returns INTERNAL_ERROR with the storage cause so the HTTP boundary logs the failure once while access remains rejected.
