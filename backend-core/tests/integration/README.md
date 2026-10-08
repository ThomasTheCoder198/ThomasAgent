# Integration tests

## Purpose
HTTP tests that cross layers and domains: real login, session, CSRF, rate limiting, event streams and the provider API.

## Entry points
- `auth_*_test.go`: auth flows
- `llmprovider_*_test.go`: provider routes with a real login and the internal resolve route

## Dependencies
- Uses: every domain, Docker Postgres and Redis
- Used by: CI (`task test:core`)

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -count=1
```

## Conventions
Unit tests belong next to their code; only tests that need several layers wired together go here.
