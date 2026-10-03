# Config

## Purpose
Loads typed startup configuration from CORE_ environment variables and validates required values and retry policy.
Local commands also read `.env` in their working directory. Process variables
take precedence; loading a file never modifies the process environment.

## Entry points
- `LoadSchema() (SchemaConfig, error)` loads only schema-generation settings,
  without requiring the application's Postgres or Redis URLs.
- `BreakerConfig` supplies typed provider circuit breaker settings.
- `Load() (Config, error)` — called during core startup.
- `Config`, `RetryConfig`, `StreamConfig`, `RelayConfig` — configuration consumed by platform services.

## Dependencies
- Uses: `github.com/caarlos0/env/v11`, platform logging's `ParseLevel`, and Go standard library.
- Used by: core startup and platform adapters added in subsequent tasks.

## Run & test
```bash
cd backend-core
go test ./internal/platform/config/... -count=1
```

## Conventions
Set `CORE_ENV_FILE` to an explicit dotenv path, or to an empty value to disable
file loading. A missing default `.env` is allowed for container deployments;
a missing explicitly configured file or malformed file fails startup. Parser
errors do not expose file contents. `backend-core/.env.example` lists local defaults;
the real `.env` is ignored by Git and excluded from Docker build contexts.

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
| CORE_STREAM_MAX_DELIVERIES | 5 |
| CORE_STREAM_VISIBILITY_TIMEOUT | 30s |
| CORE_STREAM_BLOCK_TIMEOUT | 5s |
| CORE_STREAM_BATCH_SIZE | 16 |
| CORE_RELAY_BATCH_SIZE | 100 |
| CORE_RELAY_POLL_INTERVAL | 500ms |
| CORE_SCHEMA_POSTGRES_IMAGE | postgres:18.6-alpine |
| CORE_SCHEMA_GENERATION_TIMEOUT | 2m |
| CORE_SCHEMA_CLEANUP_TIMEOUT | 15s |

`Load` returns startup errors without logging. Retry maximum delay must be at least the base delay; retry attempts, stream deliveries, breaker failure threshold and half-open call limit must be at least one.

Log levels are validated through `logging.ParseLevel`, the same parser used to construct loggers; unknown names fail startup.
Schema-generation image must be nonempty; generation and cleanup timeouts must
be positive. `Config.Schema` and `LoadSchema` use the same typed defaults and validation.

## Common failures
- Missing or empty database/Redis URL → set both required variables.
- Invalid duration or integer → use Go duration syntax or an integer.
- Unknown log level → choose debug, info, warn, or error.
- Retry maximum delay below base delay → increase CORE_RETRY_MAX_DELAY.
