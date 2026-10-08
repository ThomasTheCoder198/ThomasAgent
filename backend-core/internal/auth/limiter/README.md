# Login limiter

## Purpose
Redis budgets for login attempts per email+IP, per email and per IP, counted atomically by one Lua script.

## Entry points
- `LoginLimiter`, `NewLoginLimiter`, `Check`, `ResetLogin`, `Identity` (`login_limiter.go`)

## Dependencies
- Uses: Redis, `auth/model`, `platform/config`
- Used by: `auth/controller`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/auth/limiter -count=1
```

## Conventions
Emails are stored only as HMACs keyed by `CORE_AUTH_LOGIN_LIMITER_KEY`. Redis failure rejects the login rather than disabling the limit.
