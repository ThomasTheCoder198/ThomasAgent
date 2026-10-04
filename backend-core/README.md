# Backend core

## Purpose
Go API and agent runtime for ThomasAgent.

## Entry points
- `cmd/core`: `serve|migrate|outbox-relay` command family. Implements serve, migrate, outbox-relay, and healthcheck.
- GET `/healthz`: liveness. GET `/readyz`: injected dependency checks.
- `/api/v1/auth/login`, `/logout`, `/me`: owner sessions, cookie policy and CSRF.
- `/api/v1/providers`, `/models`, `/model-roles`: vaulted provider keys, model catalog and role assignments.
- GET `/internal/models/resolve`: service-token authentication; decrypted keys are internal-only.

## Dependencies
- Uses platform config, `internal/errors`, httpx, logging and tracing; see each README in `internal/platform/*`.
- Used by the frontend and service clients.

## Run & test
Run core through Compose. To run core outside Docker, export the CORE_* variables yourself; CORE_DATABASE_URL and CORE_REDIS_URL are required.

Database changes use hand-written Goose SQL files under `migrations/`. Add a
new timestamped migration, verify Up and Down in dev, then run `task migrate`.

Go errors are declared in generated `internal/errors/errors.go`: use named
values such as `errors.ErrNotFound` and `.WithCause(err)`. Add codes/messages in
`contracts/errors.yaml` and run `task gen`. The core executable's responsibilities
are split under `cmd/core`.

```bash
cd backend-core
# Set CORE_DATABASE_URL and CORE_REDIS_URL to reachable services.
go run ./cmd/core migrate up
go run ./cmd/core serve
go run ./cmd/core outbox-relay
go run ./cmd/core healthcheck
go test ./... -count=1 -p 2
golangci-lint fmt ./...
golangci-lint run ./...
```

## Conventions
See the [shared naming glossary](../docs/glossary.md) for terms used across services.
Go 1.27.1 is pinned. CORE_ variables configure startup. Optional CORE_OTLP_ENDPOINT enables OTLP HTTP traces and logs. Serve opens and pings Postgres and Redis before listening; readiness checks both dependencies. All HTTP responses use the shared response.
Process, HTTP, and worker logs follow the shared [log field contract](internal/platform/logging/README.md#log-field-contract).

## Common failures
- Missing database/Redis environment values fail config loading.
- An occupied HTTP address prevents startup.
- An unreachable OTLP collector prevents telemetry delivery.

## Identity and provider configuration

M0.2 users, sessions, secrets, providers, models and roles are scoped to the
[Platform tenant](../docs/glossary.md) (`default`); M1 business repositories must
derive `tenant_id` from trusted context. Owner bootstrap is idempotent.

`CORE_VAULT_MASTER_KEY` is required and must decode to 32 bytes; `serve` validates
it before opening dependencies or listening. Keep the key stable. All config-loading
commands also require `CORE_SERVICE_TOKEN`. Set `CORE_AUTH_OWNER_EMAIL` and
`CORE_AUTH_OWNER_PASSWORD` (at least 12 characters) to bootstrap the owner.
Session/cookie/login settings are documented in [config](internal/platform/config/README.md).

Provider keys are write-only; public DTOs expose `hasApiKey`. Only service-token
protected internal resolve returns a decrypted key. Provider catalog calls use
retry/backoff, circuit breaking and a guarded HTTP client. Public destinations
require HTTPS. Configure `CORE_PROVIDER_PRIVATE_ALLOWLIST` with exact `host:port`
entries for local providers; private endpoints and HTTP are denied by default.
Redirects are denied. `CORE_PROVIDER_HTTP_TIMEOUT` defaults to 20s and
`CORE_PROVIDER_MAX_RESPONSE_BYTES` to 8388608. See [outbound](internal/platform/outbound/README.md).

Run `task gen` before Go tests to validate OpenAPI and generate Postman. The
generated collection is exercised against isolated Postgres/Redis and a real HTTP
server by `TestAPI_GeneratedPostmanCollection`; test fixtures never alter existing
platform model roles. `task smoke` additionally checks deployed login/CSRF/provider
creation with cleanup. `task api-test` supports an explicit test base URL.
