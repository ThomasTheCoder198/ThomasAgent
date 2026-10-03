# Redis adapter

## Purpose
Owns Redis URL parsing, startup ping, and the readiness ClientPinger adapter.
Stream operations belong to the jobs module.

## Entry points
- `Open(ctx, url)`: create and ping a go-redis client.
- `ClientPinger.Ping(ctx)`: adapt Redis to the HTTP readiness interface.

## Dependencies
- Uses: go-redis v9, `internal/errors`, OpenTelemetry.
- Used by: core serve and outbox-relay startup; HTTP readiness.
- The caller supplies CORE_REDIS_URL through typed platform config.
- The caller owns client shutdown after a successful Open.

## Run & test
```bash
cd backend-core
go test ./... -count=1
go run ./cmd/core serve
go run ./cmd/core outbox-relay
```
Set CORE_DATABASE_URL and CORE_REDIS_URL first.
Docker Desktop is required for the jobs integration tests.

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
Failed startup ping closes the client before returning an application error.
Successful readiness ping returns a nil error interface; only failed commands are wrapped.
Open and readiness ping use the named `core.redis.open` and `core.redis.ping` client spans; startup failures are logged once by
core with a catalog code and trace context, without the connection URL.
Redis URL credentials and document content must never be logged.
Tunables for jobs come from config.OutboxRelayConfig and config.ConsumerConfig.

## Common failures
- Invalid URL: correct CORE_REDIS_URL before startup.
- Unreachable Redis: check Redis health, host, port, and credentials.
- Readiness ping failure: readiness returns unavailable until Redis recovers.
