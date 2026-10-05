# Auth

## Purpose

Bootstraps the operator, verifies Argon2id passwords, issues hashed sessions, and serves cookie-based login, logout and current-user endpoints.
M0.2 data belongs to the [Platform tenant](../../../docs/glossary.md) (`default`). Repository writes and reads require `tenant_id` from trusted server context through `tenant.ID`, which returns an error when scope is absent. Login and startup bootstrap explicitly establish platform scope; session middleware derives scope from the stored session row. Client and model parameters never select a tenant.

## Entry points

- `NewRepository`, `NewService`, `EnsureOwner`, `Login`, `Authenticate`, and `Logout`.
- `NewHandler(...).Mount` registers POST `/api/v1/auth/login`, POST `/api/v1/auth/logout`, and GET `/api/v1/auth/me`.
- `RequireSession` authenticates cookies, checks CSRF on unsafe methods and exposes `UserFrom` to protected handlers; `NewLimiter` counts login attempts.
- `PurgeAllExpiredSessions` is the explicit system-job repository/service entry point; `PurgeExpiredSessions` remains tenant-scoped for request use.
- `TracerName` and the auth span constants in `tracing.go` own fixed telemetry names, including the exported `SessionPurgeSpan` used by core.

## Dependencies

- Uses: Postgres, Redis, typed `config.AuthConfig`, Argon2id, UUIDs, catalog errors, audit recording, chi and shared httpx boundaries.
- Used by: core HTTP routing and the periodic expired-session purge job.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth -count=1
```

Tests use `pgtest.Start`, migrated Docker Postgres and Redis, with Ryuk disabled on this host. Real HTTP stream gates exercise tracing, access logging, session cookies and CSRF checks, three incremental frames, disconnect cancellation and committed-panic recovery without JSON corruption.

## Conventions

Passwords, raw session tokens and secrets must never enter logs, errors or audit metadata. Errors propagate to the boundary for one log and shared HTTP formatting.

`thomas_session` is HttpOnly; `thomas_csrf` is readable by the browser. Both use Path `/`, SameSite Lax and the constructor's Secure flag. Their absolute expiry matches the session. Unsafe methods require `X-CSRF-Token` equal to the authenticated session's token with constant-time comparison; GET, HEAD and OPTIONS are safe. Logout requires CSRF, deletes the session and clears both cookies.

The Redis limiter atomically increments expiring email/IP, email-global and IP-only counters, repairs lost TTL, and returns the longest blocked bucket's remaining TTL in `Retry-After`. The typed limits are `CORE_AUTH_LOGIN_MAX_ATTEMPTS` (10), `CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS` (20), and `CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS` (100), each at least one. The IP-only budget blocks one address spraying different emails. Identities use keyed HMAC-SHA256 of normalized email, with the injected service-token secret. Successful login resets the email/IP and email-global counters; the aggregate IP budget remains until expiry. Redis/script failures reject the login with INTERNAL_ERROR before password validation. Rotation of the HMAC key resets email budgets.

The email-global bucket intentionally runs before password verification and still blocks a correct password once exhausted. Someone who knows the owner email can therefore lock that account out for the remaining window, including by rotating IPs. This retains brute-force protection across addresses; there is no successful-password exemption. The IP-only budget reduces spraying from a single address but cannot eliminate a distributed account lockout. Shared NAT addresses also share the IP budget; tune its higher default for trusted deployment traffic.

Forwarding headers default to disabled. With an explicitly configured header and trusted proxy CIDRs, only trusted RemoteAddr peers can forward an address; all header lines are joined in received order before traversal chooses the rightmost untrusted hop. Invalid forwarding chains fall back to RemoteAddr. The trusted edge proxy must append the actual connecting address.

Ordinary credential failures produce a warning with trace_id and HMAC identity, without writing audit rows. The email counter's first blocked attempt records at most one credential-free audit event per window; audit failure cannot alter invalid-credential responses or the limiter response. Successful session creation and its audit row share one transaction; rollback failures preserve both causes.

Argon2 creation and comparison acquire a bounded semaphore using request cancellation. Cancelled waits return retryable DEPENDENCY_TIMEOUT errors. Iterations, memory, parallelism, salt length, key length and concurrency come from typed config; running hashes release their slot after completion.

Middleware preserves the response writer and request cancellation. Session reads compute `Expired` using database `now()` and use that same clock for last-seen refresh and purge. App-clock skew cannot override the database expiry decision. Session reads update last_seen_at only after CORE_AUTH_SESSION_TOUCH_INTERVAL and never for rows whose expiry has passed. The token lookup establishes trusted scope before the scoped session query. The periodic system job deletes expired sessions across every tenant through `PurgeAllExpiredSessions`, preserving active sessions; request-path reads/deletes and `PurgeExpiredSessions` still require tenant context. Limiter/hash operations and the purge job carry parented spans.

## Common failures

- An empty owner email or short bootstrap password requires CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD. Expired or missing sessions return catalog auth errors.
- AUTH_CSRF_INVALID means the unsafe request omitted or mismatched X-CSRF-Token; RATE_LIMITED means an email/IP, email-global or IP-only attempt limit was exceeded. Login errors use the same response for unknown email and wrong password; `/me` exposes only ID, email and display name.

Emails are normalized before lookup. Only SHA-256 token hashes are stored; CSRF tokens accompany issued sessions. Session creation and successful-login audit recording share one transaction. Failed logins are logged without credentials; only the first account-limit transition is audited. Session lifetime, Argon2id parameters, hash concurrency, trusted-proxy header/CIDRs and bootstrap password length come from typed config. Bootstrap holds a transaction advisory lock before counting and creating users, so concurrent startups with different emails still create one owner. Session queries explicitly scope the trusted tenant, throttle `last_seen_at` refresh, and the service periodically purges expired rows. Login and `/me` responses use `Cache-Control: no-store`.
