# Redis adapter

## Purpose
Owns Redis URL parsing, startup ping, and the readiness Pinger adapter.
Stream operations belong to the jobs module.

## Entry points
- `Open(ctx, url)`: create and ping a go-redis client.
- `Pinger.Ping(ctx)`: adapt Redis to the HTTP readiness interface.

## Dependencies
- Uses: go-redis v9, apperr, OpenTelemetry.
- Used by: core serve and relay startup; HTTP readiness.
- The caller supplies CORE_REDIS_URL through typed platform config.
- The caller owns client shutdown after a successful Open.

## Run & test
```bash
cd backend-core
go test ./... -count=1
go run ./cmd/core serve
go run ./cmd/core relay
```
Set CORE_DATABASE_URL and CORE_REDIS_URL first.
Docker Desktop is required for the jobs integration tests.

## Conventions
Failed startup ping closes the client before returning an application error.
Open and readiness ping use client spans; startup failures are logged once by
core with a catalog code and trace context, without the connection URL.
Redis URL credentials and document content must never be logged.
Tunables for jobs come from config.RelayConfig and config.StreamConfig.

## Common failures
- Invalid URL: correct CORE_REDIS_URL before startup.
- Unreachable Redis: check Redis health, host, port, and credentials.
- Readiness ping failure: readiness returns unavailable until Redis recovers.
