# Config

## Purpose
Loads typed startup configuration from CORE_ environment variables and validates required values and retry policy.

## Entry points
- `Load() (Config, error)` — called during core startup.
- `Config`, `RetryConfig`, `StreamConfig`, `RelayConfig` — configuration consumed by platform services.

## Dependencies
- Uses: `github.com/caarlos0/env/v11` and Go standard library.
- Used by: core startup and platform adapters added in subsequent tasks.

## Run & test
```bash
cd backend-core
go test ./internal/platform/config/... -count=1
```

## Conventions
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
| CORE_STREAM_MAX_DELIVERIES | 5 |
| CORE_STREAM_VISIBILITY_TIMEOUT | 30s |
| CORE_STREAM_BLOCK_TIMEOUT | 5s |
| CORE_STREAM_BATCH_SIZE | 16 |
| CORE_RELAY_BATCH_SIZE | 100 |
| CORE_RELAY_POLL_INTERVAL | 500ms |

`Load` returns startup errors without logging. Retry maximum delay must be at least the base delay; retry attempts and stream deliveries must be at least one.

## Common failures
- Missing or empty database/Redis URL → set both required variables.
- Invalid duration or integer → use Go duration syntax or an integer.
- Unknown log level → choose debug, info, warn, or error.
- Retry maximum delay below base delay → increase CORE_RETRY_MAX_DELAY.
