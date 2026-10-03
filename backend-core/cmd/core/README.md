# Core command

## Purpose

`cmd/core` is the entrypoint that assembles configuration, logging, telemetry and
platform clients, then starts the command chosen on the CLI. Domain features
belong in `internal/`; this folder owns process startup and shutdown.

## Entry points

`main.go` only calls `runProcess` and translates its result to the process exit
code. The work is divided by responsibility so startup can be read without
mixing it with HTTP, migration or worker logic.

| File | Responsibility |
| --- | --- |
| `main.go` | Enter the executable and return its exit code after cleanup finishes. |
| `command.go` | `runProcess` handles signals and exit errors; `executeCommand` validates and dispatches the CLI command. |
| `runtime.go` | `newApplication` assembles config, logger and telemetry; `shutdownTelemetry` bounds cleanup by the configured timeout. |
| `dependencies.go` | `openPostgres` and `openRedis` open clients; `recordDependencyFailure` sanitizes diagnostics and logs failures once. |
| `server.go` | `serveHTTP` starts the API, mounts health routes and shuts HTTP down; `readinessChecks` supplies dependency pings. |
| `migrate.go` | `applyMigrations` applies or rolls back migrations; `reportMigrationStatus` logs each returned status row. |
| `relay.go` | `runOutboxRelay` starts the worker that publishes committed outbox events to Redis Streams. |
| `healthcheck.go` | `checkHTTPHealth` checks the local HTTP liveness endpoint for Docker. |

The same compiled `core` executable serves three roles in Compose: `migrate`
exits after preparing the database, `serve` runs the API, and `relay` runs the
outbox worker. Separate processes give the API and worker independent lifetimes;
Compose starts them only after migration succeeds. `healthcheck` is a short
probe launched by Docker against the API process.

## Dependencies

- Uses: platform config, logging, tracing, httpx, postgres, redisx, jobs, and embedded migrations.
- Used by: the backend Docker image, Compose `core`/`migrate`/`relay` services, and local CLI commands.

## Run & test

```bash
cd backend-core
cp .env.example .env
# Fill CORE_DATABASE_URL and CORE_REDIS_URL for your local services.
go run ./cmd/core migrate up
go run ./cmd/core migrate status
go run ./cmd/core migrate down
go run ./cmd/core serve
go run ./cmd/core relay
go run ./cmd/core healthcheck
go test ./cmd/core -count=1
```

Local configuration comes from `.env` in the working directory and process
environment variables; process environment values take precedence, including
explicitly empty values. `CORE_ENV_FILE` selects another file; an explicitly
empty `CORE_ENV_FILE` disables file loading. A missing default `.env` is allowed,
but a missing explicitly selected file fails startup. Compose
configuration is supplied through `deploy/compose/.env` and service environment
blocks, so it does not use the local backend `.env` file.

## Conventions

Missing or unknown commands report CLI usage before configuration or dependencies
are initialized. `healthcheck` reads the configured HTTP address and checks
`/healthz` without opening database clients. `serve` opens and pings Postgres and
Redis before listening and includes both in `/readyz`.

HTTP and telemetry shutdown use `CORE_SHUTDOWN_TIMEOUT` deadlines so unavailable
collectors cannot hold process exit indefinitely. Listening logs use the active
context. An interrupt or termination signal cancels the API or relay work.

`migrate` defaults to `up`; `down` rolls back one migration; failures exit with
code 1. `status` logs version, path and applied/pending state from
`postgres.Status`. Migration and startup connection/ping run inside client spans
(`postgres.migrate`, `postgres.open`, `redis.startup`/`redis.open`).

Dependency failures emit one structured error with the catalog code, trace
context and underlying diagnostics after removing dependency URLs and decoded
passwords. The span records the same sanitized error. The process boundary
recognizes an already logged error and returns a failure exit code without
printing a second copy. Logger creation rejects unknown log levels.

## Common failures

- Missing required environment variables or invalid configuration values fail startup.
- Unreachable Postgres or Redis fails startup or readiness; an occupied HTTP port fails listening.
- Invalid migration SQL or an unknown migration direction makes `migrate` fail and blocks the Compose API/relay startup gate.
