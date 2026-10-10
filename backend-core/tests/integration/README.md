# Integration tests

## Purpose
HTTP tests that cross layers and domains: real login, session, CSRF, rate limiting, event streams, the provider API and organization provisioning.

## Entry points
- `auth_*_test.go`: auth flows
- `llmprovider_*_test.go`: provider routes with a real login
- `organization_http_test.go`: organization routes against a fake ragx; `ragx_fake_test.go` is that fake (it verifies every App token with the shared vector key from `contracts/app-token.json`, like ragx, and is reused by the proxy tests)

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
