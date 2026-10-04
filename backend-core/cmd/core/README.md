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
| `application.go`  | `newApplication` assembles config, logger and telemetry; `shutdownTelemetry` bounds cleanup by the configured timeout.                                              |
| `connections.go`  | `openPostgres` and `openRedis` open clients; `recordDependencyFailure` sanitizes diagnostics and logs failures once.                                                |
| `server.go`       | `serveHTTP` starts the API, mounts health routes and shuts HTTP down; `readinessChecks` supplies dependency pings.                                                  |
| `router.go`       | Mounts authentication, protected Registry APIs, internal resolve and health routes. Ordinary routes carry request deadlines; future stream routes mount separately. |
| `migrate.go`      | `applyMigrations` applies or rolls back migrations; `reportMigrationStatus` logs each returned status row.                                                          |
| `outbox_relay.go` | `runOutboxRelay` starts the worker that publishes committed outbox events to Redis Streams.                                                                         |
| `healthcheck.go`  | `checkHTTPHealth` checks the local HTTP liveness endpoint for Docker.                                                                                               |

The same compiled `core` executable serves three roles in Compose: `migrate`
exits after preparing the database, `serve` runs the API, and `outbox-relay` runs the
outbox worker. Separate processes give the API and worker independent lifetimes;
Compose starts them only after migration succeeds. `healthcheck` is a short
probe launched by Docker against the API process.

## Dependencies

- Uses: `internal/errors`, platform config, logging, tracing, httpx, postgres, redisx, jobs, and embedded migrations.
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

Core reads process environment variables. Compose supplies them through
`deploy/compose/.env` and service environment blocks. Export CORE_* variables
yourself when running the binary outside Docker.

## Conventions
See the [shared naming glossary](../../../docs/glossary.md) for terms used across services.

Missing or unknown commands report CLI usage before configuration or dependencies
are initialized. `healthcheck` reads the configured HTTP address and checks
`/healthz` without opening database clients. `serve` opens and pings Postgres and
Redis before listening and includes both in `/readyz`.

HTTP and telemetry shutdown use `CORE_SHUTDOWN_TIMEOUT` deadlines so unavailable
collectors cannot hold process exit indefinitely. Listening logs use the active
context. An interrupt or termination signal cancels the API or outbox relay work.

`migrate` defaults to `up`; `down` rolls back one migration; failures exit with
code 1. `status` logs version, path and applied/pending state from
`postgres.MigrationStatuses`. Migration and startup connection/ping run inside client spans
(`core.postgres.migrate`, `core.postgres.open`, `core.redis.startup`/`core.redis.open`).

Dependency failures emit one structured error with the catalog code, trace
context and underlying diagnostics after removing dependency URLs and decoded
passwords. The span records the same sanitized error. The process boundary
recognizes an already logged error and returns a failure exit code without
printing a second copy. Logger creation rejects unknown log levels.
`newApplication` passes `Config.Environment` into the logger; process and HTTP logs follow the shared [log field contract](../../internal/platform/logging/README.md#log-field-contract).

## Common failures

- Missing required environment variables or invalid configuration values fail startup.
- Unreachable Postgres or Redis fails startup or readiness; an occupied HTTP port fails listening.
- Invalid migration SQL or an unknown migration direction makes `migrate` fail and blocks the Compose API/outbox-relay startup gate.

All commands that load configuration, including `healthcheck`, require the nonempty
`CORE_VAULT_MASTER_KEY`. Generate a base64 32-byte key with `openssl rand -base64 32`;
keep it stable to preserve access to stored secrets. `CORE_VAULT_KEY_ID` defaults to `v1`.

Core startup configures M0.2 platform-scoped modules. Their data belongs to the [Platform tenant](../../../docs/glossary.md) (`default`). M1 business tenancy must derive `tenant_id` from trusted context, never client or model parameters.

All config-loading commands also require nonempty CORE_SERVICE_TOKEN. The identity test fixtures supply it explicitly.

`serve` validates the vault key before dependency access, bootstraps the owner,
then injects the guarded provider catalog into the Registry. The route coverage
test checks every mounted method/path against the shared OpenAPI contract.
The generated Postman collection runs against a live HTTP server backed by
disposable migrated Postgres and Redis in `TestAPI_GeneratedPostmanCollection`.
Its fixtures never change the existing platform roles or read real credentials.
