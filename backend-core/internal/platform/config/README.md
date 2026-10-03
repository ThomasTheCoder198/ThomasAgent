# Config

## Purpose
Loads typed startup configuration from CORE_ environment variables and validates required values and retry policy.

## Entry points
- `BreakerConfig` supplies typed provider circuit breaker settings.
- `Load() (Config, error)` — called during core startup.
- `Config`, `RetryConfig`, `ConsumerConfig`, `OutboxRelayConfig` — configuration consumed by platform services. `Config.OutboxRelay` retains the `CORE_RELAY_` environment prefix and its existing defaults.

## Dependencies
- Uses: `github.com/caarlos0/env/v11`, platform logging's `ParseLevel`, and Go standard library.
- Used by: core startup and platform adapters added in subsequent tasks.

## Run & test
```bash
cd backend-core
go test ./internal/platform/config/... -count=1
```

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
Only process environment variables are read. Compose supplies them from
`deploy/compose/.env` and its service environment blocks.

Environment variables and defaults:

| Variable | Default |
|---|---|
| CORE_ENV | dev |
| CORE_HTTP_ADDR | :8080 |
| CORE_SHUTDOWN_TIMEOUT | 15s |
| CORE_READ_HEADER_TIMEOUT | 10s |
| CORE_DATABASE_URL | Required, nonempty |
| CORE_REDIS_URL | Required, nonempty |
| CORE_OTLP_ENDPOINT | Empty; disables export |
| CORE_LOG_LEVEL | info; debug/info/warn/error |
| CORE_SERVICE_NAME | thomas-core |
| CORE_RETRY_MAX_ATTEMPTS | 4 |
| CORE_RETRY_BASE_DELAY | 200ms |
| CORE_RETRY_MAX_DELAY | 10s |
| CORE_BREAKER_FAILURE_THRESHOLD | 5 |
| CORE_BREAKER_OPEN_TIMEOUT | 30s |
| CORE_BREAKER_HALF_OPEN_MAX_CALLS | 1 |
| CORE_STREAM_MAX_DELIVERIES | 4 (initial delivery + 3 retries) |
| CORE_STREAM_VISIBILITY_TIMEOUT | 30s |
| CORE_STREAM_BLOCK_TIMEOUT | 5s |
| CORE_STREAM_BATCH_SIZE | 16 |
| CORE_RELAY_BATCH_SIZE | 100 |
| CORE_RELAY_POLL_INTERVAL | 500ms |

`Load` returns startup errors without logging. Retry maximum delay must be at least the base delay; retry attempts, stream deliveries, breaker failure threshold and half-open call limit must be at least one.

Log levels are validated through `logging.ParseLevel`, the same parser used to construct loggers; unknown names fail startup.

The logger's field contract is defined in the [logging README](../logging/README.md#log-field-contract). `Config.Environment` supplies every logger's deployment environment.

## Common failures
- Missing or empty database/Redis URL → set both required variables.
- Invalid duration or integer → use Go duration syntax or an integer.
- Unknown log level → choose debug, info, warn, or error.
- Retry maximum delay below base delay → increase CORE_RETRY_MAX_DELAY.
