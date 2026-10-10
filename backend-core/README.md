# Backend core

## Purpose
Go API and agent runtime for ThomasAgent.

## Entry points
- `cmd/core`: `serve|migrate|outbox-relay` command family. Implements serve, migrate, outbox-relay, and healthcheck. `serve` needs the App signing key.
- GET `/healthz`: liveness. GET `/readyz`: injected dependency checks.
- `/api/v1/auth/login`, `/logout`, `/me`: owner sessions, cookie policy and CSRF.
- `/api/v1/providers`, `/models`, `/model-roles`: vaulted provider keys, model catalog and role assignments.
- `/api/v1/orgs`, `/orgs/{orgId}`, `/orgs/{orgId}/provision`: organizations, provisioned into ragx synchronously (see `internal/organization`).
- `/api/v1/orgs/{orgId}/apps/ragx/{path}`: generic proxy to ragx's `/v1` API for GET, HEAD, POST, PUT, PATCH and DELETE (see `internal/appproxy`).

## Dependencies
- Uses platform config, `internal/errors`, httpx, logging, telemetry and requestid; see each README in `internal/platform/*`.
- Used by the frontend and service clients.

## Run & test
Run core through Compose. Outside Docker, core reads `.env` in its working directory or the file named by `CORE_ENV_FILE`, then applies exported CORE_* overrides. CORE_DATABASE_URL and CORE_REDIS_URL must point to reachable services.

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
# serve also needs CORE_APP_SIGNING_KEY_FILE, e.g. ../deploy/ragx/dev-app-signing-key.pem (create it with `task ragx:dev-key`).
go run ./cmd/core serve
go run ./cmd/core outbox-relay
go run ./cmd/core healthcheck
go test ./... -count=1 -p 2
golangci-lint fmt ./...
golangci-lint run ./...
```

## Conventions
See the [shared naming glossary](../docs/glossary.md) for terms used across services.
Go 1.27.1 is pinned. Non-secret defaults are in `internal/platform/config/config.yaml`; dotenv and process variables override them. Optional CORE_LOGS_OTLP_ENDPOINT (full logs URL) enables OTLP HTTP log export. Core currently has no spans and correlates by `request_id`; minimal HTTP-to-LLM tracing is planned with the M2 runtime. Serve opens and pings Postgres and Redis before listening; readiness checks both dependencies. All HTTP responses use the shared response.
Process, HTTP, and worker logs follow the shared [log field contract](internal/platform/logging/README.md#log-field-contract).

## Common failures
- Missing database/Redis environment values fail config loading.
- An occupied HTTP address prevents startup.
- An unreachable logs endpoint drops exported records; stdout logs are unaffected.

## Identity and provider configuration

M0.2 users, sessions, secrets, providers, models and roles are scoped to the
[Platform tenant](../docs/glossary.md) (`default`); M1 business repositories must
derive `tenant_id` from trusted context. Owner bootstrap is idempotent.

`CORE_VAULT_MASTER_KEY` is required and must decode to 32 bytes; `serve` validates
it before opening dependencies or listening. Keep the key stable. `serve` also requires `CORE_APP_SIGNING_KEY` or `CORE_APP_SIGNING_KEY_FILE` (never both) and checks it before opening dependencies; `task core:serve` points at `deploy/ragx/dev-app-signing-key.pem`. All config-loading
commands also require `CORE_SERVICE_TOKEN` and a different `CORE_AUTH_LOGIN_LIMITER_KEY`, each of at least 32 characters; `change-me*` values are rejected outside dev. `CORE_SERVICE_TOKEN` is currently unused (reserved; its `/internal` role was removed in M1.2) but config validation still requires it. Set `CORE_AUTH_OWNER_EMAIL` and
`CORE_AUTH_OWNER_PASSWORD` (at least 12 characters) to bootstrap the owner.
Session/cookie/login settings are documented in [config](internal/platform/config/README.md).

Provider keys are write-only; public DTOs expose `hasApiKey`. No HTTP route returns a decrypted key;
the M2 agent runtime reads it through `ResolveService`. Provider catalog calls use
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

M0.2 hardening requires explicit tenant context at all business repositories; database tenant defaults have been removed by a new migration. Login uses keyed account and address budgets, hashing waits honor cancellation, and session last-seen writes are throttled. JSON endpoints return 415 for unsupported content types. See module READMEs for configuration and regression gates.

Review round 3 adds the IP-only login budget, retries live-parent attempt timeouts, makes the system session purge cover all tenants, and uses database session expiry. Vault writes version 3 length-prefixed AAD while upgrading prior formats on read. Run Docker-backed packages one at a time with `TESTCONTAINERS_RYUK_DISABLED=true`; `task test` uses Go `-p 1` to serialize packages and must finish before builds or `task up`.
