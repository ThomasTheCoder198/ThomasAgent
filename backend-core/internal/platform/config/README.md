# Config

## Purpose
Loads typed startup configuration from CORE_ environment variables and validates required values and retry policy.

## Entry points
- `BreakerConfig` supplies typed provider circuit breaker settings.
- `Load() (Config, error)` — called during core startup.
- `Config`, `HTTPConfig`, `RetryConfig`, `ConsumerConfig`, `OutboxRelayConfig` — configuration consumed by platform services. `Config.OutboxRelay` retains the `CORE_RELAY_` environment prefix and its existing defaults.

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

| Variable                                  | Default                                                      |
| ----------------------------------------- | ------------------------------------------------------------ |
| CORE_ENV                                  | dev                                                          |
| CORE_HTTP_ADDR                            | :8080                                                        |
| CORE_HTTP_MAX_BODY_BYTES                  | 1048576 (1 MiB); must be >= 1                                |
| CORE_HTTP_REQUEST_TIMEOUT                 | 30s; must be > 0                                             |
| CORE_HTTP_EVENT_STREAM_HEARTBEAT          | 15s; must be > 0                                             |
| CORE_HTTP_EVENT_STREAM_WRITE_TIMEOUT      | 10s; must be > 0                                             |
| CORE_SHUTDOWN_TIMEOUT                     | 15s                                                          |
| CORE_READ_HEADER_TIMEOUT                  | 10s                                                          |
| CORE_DATABASE_URL                         | Required, nonempty                                           |
| CORE_REDIS_URL                            | Required, nonempty                                           |
| CORE_VAULT_MASTER_KEY                     | Required, nonempty; base64 of exactly 32 bytes for the vault |
| CORE_VAULT_KEY_ID                         | v1                                                           |
| CORE_OTLP_ENDPOINT                        | Empty; disables export                                       |
| CORE_LOG_LEVEL                            | info; debug/info/warn/error                                  |
| CORE_SERVICE_NAME                         | thomas-core                                                  |
| CORE_RETRY_MAX_ATTEMPTS                   | 4                                                            |
| CORE_RETRY_BASE_DELAY                     | 200ms                                                        |
| CORE_RETRY_MAX_DELAY                      | 10s                                                          |
| CORE_PROVIDER_BREAKER_FAILURE_THRESHOLD            | 5                                                            |
| CORE_PROVIDER_BREAKER_OPEN_TIMEOUT                 | 30s                                                          |
| CORE_PROVIDER_BREAKER_HALF_OPEN_MAX_CALLS          | 1                                                            |
| CORE_PROVIDER_HTTP_TIMEOUT                | 20s; must be > 0                                             |
| CORE_PROVIDER_MAX_RESPONSE_BYTES          | 8388608 (8 MiB); must be >= 1                                |
| CORE_PROVIDER_PRIVATE_ALLOWLIST           | Empty; comma-separated exact hostname:port destinations      |
| CORE_STREAM_MAX_DELIVERIES                | 4 (initial delivery + 3 retries)                             |
| CORE_STREAM_VISIBILITY_TIMEOUT            | 30s                                                          |
| CORE_STREAM_BLOCK_TIMEOUT                 | 5s                                                           |
| CORE_STREAM_BATCH_SIZE                    | 16                                                           |
| CORE_RELAY_BATCH_SIZE                     | 100                                                          |
| CORE_RELAY_POLL_INTERVAL                  | 500ms                                                        |
| CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS           | 100; must be >= 1; independent IP-only login budget          |

`Load` returns startup errors without logging. Retry maximum delay must be at least the base delay; retry attempts, stream deliveries, breaker failure threshold and half-open call limit must be at least one.

Log levels are validated through `logging.ParseLevel`, the same parser used to construct loggers; unknown names fail startup.

The logger's field contract is defined in the [logging README](../logging/README.md#log-field-contract). `Config.Environment` supplies every logger's deployment environment.

## Common failures
- Missing or empty vault master key ? generate with `openssl rand -base64 32` and set CORE_VAULT_MASTER_KEY. Format is checked when constructing the vault cipher.
- Missing or empty database/Redis URL → set both required variables.
- Invalid duration or integer → use Go duration syntax or an integer.
- Unknown log level → choose debug, info, warn, or error.
- Retry maximum delay below base delay → increase CORE_RETRY_MAX_DELAY.

Config has no tables; it configures M0.2 platform-scoped modules. Their data belongs to the [Platform tenant](../../../../docs/glossary.md) (`default`). M1 business tenancy must derive `tenant_id` from trusted context, never client or model parameters.

Auth configuration uses CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD for bootstrap, CORE_AUTH_SESSION_TTL (720h), CORE_AUTH_SESSION_PURGE_INTERVAL (1h), CORE_AUTH_COOKIE_SECURE (false), CORE_AUTH_LOGIN_MAX_ATTEMPTS (10), CORE_AUTH_LOGIN_WINDOW (15m), CORE_AUTH_MIN_PASSWORD_LENGTH (12), and CORE_AUTH_TRUSTED_PROXY_HEADER (empty by default) and CORE_AUTH_TRUSTED_PROXY_CIDRS (empty by default). Argon2id uses CORE_AUTH_PASSWORD_HASH_ITERATIONS (2, minimum 2), CORE_AUTH_PASSWORD_HASH_MEMORY_KIB (exactly 65536), CORE_AUTH_PASSWORD_HASH_PARALLELISM (1), CORE_AUTH_PASSWORD_HASH_SALT_LENGTH (16, minimum 8), CORE_AUTH_PASSWORD_HASH_KEY_LENGTH (32, minimum 16), and CORE_AUTH_PASSWORD_HASH_MAX_CONCURRENCY (4). CORE_SERVICE_TOKEN must be at least 32 characters; values beginning with `change-me` are rejected outside dev. Bootstrap credential validation belongs to the auth service when no owner exists.

Auth session TTL and login window must be positive durations; minimum password length and maximum login attempts must be at least one. These startup checks do not require optional owner bootstrap credentials when loading config; EnsureOwner validates credentials when no owner exists.

Provider configuration validates timeout, response-byte cap and breaker limits before startup. `DefaultProviderMaxResponseBytes` supplies the same 8 MiB default for environment loading and programmatic catalog construction. Private destination exceptions default to empty; they authorize HTTP and private/loopback connectivity only for exact hostname:port matches. See the [outbound module](../outbound/README.md) for URL, DNS and redirect policy. Allowlist validation never echoes malformed input.

The account-wide login limit uses CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS (20) independently of the email/IP limit. CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS (100) is a separate IP-only budget across emails, defaults above both other limits, and must be at least one. Trusted proxy CIDRs must parse successfully. CORE_AUTH_SESSION_TOUCH_INTERVAL (5m) controls last_seen_at write frequency. Provider synchronization and breaker cache size are bounded by CORE_PROVIDER_MAX_REMOTE_MODELS (1000) and CORE_PROVIDER_MAX_BREAKERS (256); both must be positive. CORE_PROVIDER_BREAKER_* is the only breaker configuration namespace; unused CORE_BREAKER_* settings are no longer read. Deployment forwarding and example validation are covered by deploy_test.go.
