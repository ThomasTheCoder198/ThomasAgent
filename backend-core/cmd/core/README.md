# Core command

## Purpose
Owns process startup, Postgres migration execution, outbox relay, and graceful HTTP shutdown.

## Entry points
- `main`, `run`, `app.serve`, `app.migrate`, `app.relay`

## Dependencies
- Uses: platform config, logging, tracing, httpx, postgres, redisx, jobs, and embedded migrations.

## Run & test
```bash
cd backend-core
# Set CORE_DATABASE_URL and CORE_REDIS_URL in the environment.
go run ./cmd/core migrate up
go run ./cmd/core migrate status
go run ./cmd/core migrate down
go run ./cmd/core serve
go run ./cmd/core relay
go test ./... -count=1
```

## Conventions
Serve opens and pings Postgres and Redis before listening and includes both in readiness.
Process exit codes are returned from realMain so deferred signal cleanup completes.
Migrate defaults to up; down rolls back one migration; errors exit with code 1.
Migration and startup connection/ping run inside client spans (`postgres.migrate`,
`postgres.open`). Failures emit one structured error with the catalog code and
trace context through the existing logger; raw database errors stay out of these
logs and span attributes. Configure the existing OTLP endpoint to export spans.
Relay opens both clients and publishes outbox batches until signal cancellation.
Redis startup uses redis.startup/redis.open spans and safe correlated failure logs.

## Common failures
- Missing required environment variables fail startup.
- Unreachable Postgres or Redis fails startup or readiness; occupied HTTP ports fail listening.
- Invalid migration SQL or an unknown direction makes migrate fail.
