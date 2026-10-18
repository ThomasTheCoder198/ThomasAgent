# Core command

## Purpose

`cmd/core` is the entrypoint that assembles configuration, logging, telemetry and
platform clients, then starts the command chosen on the CLI. Domain features
belong in `internal/`; this folder owns process startup and shutdown.

## Entry points

`main.go` only calls `runProcess` and translates its result to the process exit
code. The work is divided by responsibility so startup can be read without
mixing it with HTTP, migration or worker logic.

| File              | Responsibility                                                                                                                                                      |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `main.go`         | Enter the executable and return its exit code after cleanup finishes.                                                                                               |
| `command.go`      | `runProcess` handles signals and exit errors; `executeCommand` validates and dispatches the CLI command.                                                            |
| `runtime.go`  | `newCoreRuntime` assembles config, logger and telemetry; `shutdownTelemetry` bounds cleanup by the configured timeout.                                              |
| `connections.go`  | `openPostgres` and `openRedis` open clients; `recordDependencyFailure` sanitizes diagnostics and logs failures once.                                                |
| `container.go`    | Composition root, one container per layer like gim-core: `ClientContainer` → `RepositoryContainer` → `ServiceContainer` → `ControllerContainer`. |
| `server.go`       | `serveHTTP` opens Postgres and Redis, builds dependencies, starts the HTTP server and its background jobs. |
| `routes.go`       | Registers every path, method and per-route middleware on the Gin engine; ordinary routes carry request deadlines and future stream routes mount separately. |
| `session_purge.go` | Runs the scheduled cross-tenant expired-session purge with its own job request id. |
| `migrate.go`      | `applyMigrations` applies or rolls back migrations; `reportMigrationStatus` logs each returned status row.                                                          |
| `outbox_relay.go` | `runOutboxRelay` starts the worker that publishes committed outbox events to Redis Streams.                                                                         |
| `healthcheck.go`  | `checkHTTPHealth` checks the local HTTP liveness endpoint for Docker.                                                                                               |

Organization routes (`GET`/`POST /api/v1/orgs`, `GET`/`PATCH /api/v1/orgs/{orgId}`, `POST /api/v1/orgs/{orgId}/provision`) sit in the session group of `routes.go`. `middlewares.organizations` carries the `OrganizationLookup` that `RequireOrg` uses on org-scoped routes. The app proxy route `/api/v1/orgs/:orgId/apps/ragx/*path` (`appProxyRoute`, the six methods from `appmodel.ForwardedMethods()`) sits behind `LimitRequestDuration` → `RequireSession` (CSRF) → `RequireOrg`, in that order. The organization service takes its attempts from `CORE_RAGX_PROVISION_ATTEMPTS`, not `CORE_RETRY_MAX_ATTEMPTS`.

The business routes (org model roles, agents and conversations) are registered in the same `RequireOrg` group. `container.go` also exposes `ServiceContainer.ChatStore`, the persistence port M2b's run loop takes from the container. `api_collection_test.go` runs the generated Postman collection against an isolated real core with a small in-process fake ragx (provisioning and the collection KB read only; other KB IDs return `KB_NOT_FOUND`), so its organization ends `ready` and the Agents folder passes `RequireOrg`.

The CLI dispatches `serve` through `runProcess` ? `executeCommand` ? `serveHTTP`. Compose and the Dockerfile both invoke `core serve`. The same compiled `core` executable serves three roles in Compose: `migrate`
exits after preparing the database, `serve` runs the API, and `outbox-relay` runs the
outbox worker. Separate processes give the API and worker independent lifetimes;
Compose starts them only after migration succeeds. `healthcheck` is a short
probe launched by Docker against the API process.

## Dependencies

- Uses: `internal/errors`, platform config, logging, telemetry, httpx, postgres, redisx, jobs, and embedded migrations.
- Used by: the backend Docker image, Compose `core`/`migrate`/`outbox-relay` services, and local CLI commands.

## Run & test

```bash
cd backend-core
# Export CORE_DATABASE_URL, CORE_REDIS_URL and CORE_VAULT_MASTER_KEY for your local services.
go run ./cmd/core migrate up
go run ./cmd/core migrate status
go run ./cmd/core migrate down
go run ./cmd/core serve
go run ./cmd/core outbox-relay
go run ./cmd/core healthcheck
go test ./cmd/core -count=1
```

Core loads embedded `config.yaml` defaults, an optional `.env` file, and then
process environment variables. Compose supplies `deploy/core/.env` through
`env_file`; `task core:serve` points a host process at that file with `CORE_ENV_FILE`.
Host execution still needs reachable `CORE_DATABASE_URL` and `CORE_REDIS_URL`.

## Conventions

`serve` and `outbox-relay` start an internal `/metrics` listener on `CORE_METRICS_ADDR` (default `:9464`, explicitly empty disables). The public router does not expose it, and compose does not publish it to the host. A bind failure is logged while the process keeps running.
See the [shared naming glossary](../../../docs/glossary.md) for terms used across services.

Missing or unknown commands report CLI usage before configuration or dependencies
are initialized. `healthcheck` reads the configured HTTP address and checks
`/healthz` through loopback with its own bounded HTTP client and without opening database clients. `serve` opens and pings Postgres and
Redis before listening and includes both in `/readyz`.

`shutdownTelemetry` stays on `coreRuntime` as the teardown pair for telemetry setup in `newCoreRuntime` and uses the configured timeout. HTTP and telemetry shutdown use `CORE_SHUTDOWN_TIMEOUT` deadlines so unavailable
collectors cannot hold process exit indefinitely. Listening logs use the active
context. An interrupt or termination signal cancels the API or outbox relay work.

`migrate` defaults to `up`; `down` rolls back one migration; failures exit with
code 1. `status` logs version, path and applied/pending state from
`postgres.MigrationStatuses`. Dependency failures (migration, Postgres open, Redis ping) emit one structured error with the catalog code and a sanitized error, never URLs or passwords. The process boundary
recognizes an already logged error and returns a failure exit code without
printing a second copy. Logger creation rejects unknown log levels.
`serveHTTP` passes the injected logger to the Gin engine; `newCoreRuntime` passes `Config.Environment` into the logger; process and HTTP logs follow the shared [log field contract](../../internal/platform/logging/README.md#log-field-contract).

## Common failures

- Missing required environment variables or invalid configuration values fail startup.
- Unreachable Postgres or Redis fails startup or readiness; an occupied HTTP port fails listening.
- Invalid migration SQL or an unknown migration direction makes `migrate` fail and blocks the Compose API/outbox-relay startup gate.

All commands that load configuration, including `healthcheck`, require the nonempty
`CORE_VAULT_MASTER_KEY`. Generate a base64 32-byte key with `openssl rand -base64 32`;
keep it stable to preserve access to stored secrets. `CORE_VAULT_KEY_ID` defaults to `v1`.

Core startup configures M0.2 platform-scoped modules. Their data belongs to the [Platform tenant](../../../docs/glossary.md) (`default`). M1 business tenancy must derive `tenant_id` from trusted context, never client or model parameters.

All config-loading commands also require CORE_AUTH_LOGIN_LIMITER_KEY with at least 32 characters; values beginning `change-me` are rejected outside dev. The identity test fixtures supply it explicitly. The serve process periodically purges expired sessions across all tenants through the explicit system-level auth method and refreshes `last_seen_at` during authenticated requests. Expiry decisions use the database clock. Each purge run logs with its own `job_` request id and `duration_ms`.

`serve` validates the vault key and loads the App signing key (`CORE_APP_SIGNING_KEY` or `_FILE`; it refuses to start without one) before dependency access; `ClientContainer.Ragx` is the ragx caller built from it, bootstraps the owner,
then injects the guarded provider catalog into the LLM provider service. The route coverage
test checks every mounted method/path against the shared OpenAPI contract.
The generated Postman collection runs against a live HTTP server backed by
disposable migrated Postgres and Redis in `TestAPI_GeneratedPostmanCollection`.
Its fixtures never change the existing platform roles or read real credentials.

Bootstrap, login and the relay process establish the platform tenant explicitly. The purge job is deliberately system-wide; request repository methods remain scoped. Authenticated business requests use the tenant stored on the session row. Configured trusted proxy networks, keyed login budgets including the IP-only budget, session touch interval, catalog size and breaker cache capacity are injected at startup.
