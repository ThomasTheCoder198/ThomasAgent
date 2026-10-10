# Config

## Purpose
Loads typed startup configuration from CORE_ environment variables and validates required values and retry policy.

## Entry points
- `BreakerConfig` supplies typed provider circuit breaker settings.
- `Load() (Config, error)` — called during core startup.
- `Config`, `HTTPConfig`, `MetricsConfig`, `RetryConfig`, `ConsumerConfig`, `OutboxRelayConfig` — configuration consumed by platform services. `Config.OutboxRelay` retains the `CORE_RELAY_` environment prefix and its existing defaults.

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
`Load` reads embedded `config.yaml` as the single source of non-secret defaults. It then reads `.env` in the process working directory when present (or `CORE_ENV_FILE` when set), and finally applies process environment variables. Precedence: process environment > dotenv > config.yaml. An explicitly named missing dotenv file is an error. Compose injects `deploy/core/.env` through `env_file`; `task core:serve` names that file for a host process. Core keeps values in a per-load map and does not mutate process environment.

`CORE_DATABASE_URL`, `CORE_REDIS_URL`, `CORE_SERVICE_TOKEN`, `CORE_AUTH_LOGIN_LIMITER_KEY`, and `CORE_VAULT_MASTER_KEY` are required. Compose builds database/Redis URLs from the Postgres service values. An empty `CORE_METRICS_ADDR` disables the listener; an empty `CORE_LOGS_OTLP_ENDPOINT` disables remote log export. All other empty overrides retain a nonempty YAML default.

`Load` returns startup errors without logging. Retry maximum delay must be at least the base delay; retry attempts, stream deliveries, breaker failure threshold and half-open call limit must be at least one.

Metric buckets must be nonempty, finite, positive and strictly ascending. The metrics constructor receives the configured slices directly, keeping config independent of the Prometheus registry.

`AppConfig` (`CORE_APP_*`) configures the App token core signs for ragx: key id, issuer, audience and TTL, with defaults in `config.yaml`. The TTL must be a whole number of seconds from `MinAppTokenTTL` (1 s) to `MaxAppTokenTTL` (60 s, spec §3.1); the signer writes `exp` in whole seconds, so a fraction would be truncated. The signing key is `CORE_APP_SIGNING_KEY` (PKCS#8 PEM value) or `CORE_APP_SIGNING_KEY_FILE` (a mounted file), never both. It is not required here: `apptoken.LoadSigningKey` checks it when `serve` starts, so `migrate` and `outbox-relay` run without it.

`RagxConfig` (`CORE_RAGX_*`) configures `internal/platform/ragx`: the base URL, the connect, request (typed calls) and response-header timeouts, the cap on a ragx error body (`MAX_ERROR_BODY_BYTES`), the cap on a forwarded response (`MAX_RESPONSE_BYTES`) and `PROVISION_ATTEMPTS`. The forward uses the HTTP route's deadline. Budget rule: `PROVISION_ATTEMPTS × REQUEST_TIMEOUT + (attempts − 1) × CORE_RETRY_MAX_DELAY` must stay below `CORE_HTTP_REQUEST_TIMEOUT`, or startup fails. Defaults are in `config.yaml`. `IdleConnTimeout` is not set on the ragx transport (ruling F22).

Log levels are validated through `logging.ParseLevel`, the same parser used to construct loggers; unknown names fail startup.

The logger's field contract is defined in the [logging README](../logging/README.md#log-field-contract). `Config.Environment` supplies every logger's deployment environment.

## Common failures
- Missing or empty vault master key ? generate with `openssl rand -base64 32` and set CORE_VAULT_MASTER_KEY. Format is checked when constructing the vault cipher.
- Missing or empty database/Redis URL → set both required variables.
- Invalid duration or integer → use Go duration syntax or an integer.
- `CORE_APP_TOKEN_TTL` below 1s, above 60s or not a whole number of seconds, or both signing key variables set → startup fails; fix the value.
- `CORE_RAGX_URL` not an absolute http(s) URL, or a non-positive `CORE_RAGX_*` timeout, byte limit or attempt count, or a provisioning budget at or above the HTTP request timeout → startup fails; fix the value.
- Unknown log level → choose debug, info, warn, or error.
- Retry maximum delay below base delay → increase CORE_RETRY_MAX_DELAY.

Config has no tables; it configures M0.2 platform-scoped modules. Their data belongs to the [Platform tenant](../../../../docs/glossary.md) (`default`). M1 business tenancy must derive `tenant_id` from trusted context, never client or model parameters.

Auth configuration uses CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD for bootstrap, CORE_AUTH_SESSION_TTL (720h), CORE_AUTH_SESSION_PURGE_INTERVAL (1h), CORE_AUTH_COOKIE_SECURE (false), CORE_AUTH_LOGIN_MAX_ATTEMPTS (10), CORE_AUTH_LOGIN_WINDOW (15m), CORE_AUTH_MIN_PASSWORD_LENGTH (12), and CORE_AUTH_TRUSTED_PROXY_HEADER (empty by default) and CORE_AUTH_TRUSTED_PROXY_CIDRS (empty by default). Argon2id uses CORE_AUTH_PASSWORD_HASH_ITERATIONS (2, minimum 2), CORE_AUTH_PASSWORD_HASH_MEMORY_KIB (exactly 65536), CORE_AUTH_PASSWORD_HASH_PARALLELISM (1), CORE_AUTH_PASSWORD_HASH_SALT_LENGTH (16, minimum 8), CORE_AUTH_PASSWORD_HASH_KEY_LENGTH (32, minimum 16), and CORE_AUTH_PASSWORD_HASH_MAX_CONCURRENCY (4). CORE_SERVICE_TOKEN is currently unused (reserved; its `/internal` role was removed in M1.2) but still required by config validation, and must be at least 32 characters; values beginning with `change-me` are rejected outside dev. CORE_AUTH_LOGIN_LIMITER_KEY is the HMAC key for login-limiter identities: at least 32 characters, no `change-me` outside dev, and different from CORE_SERVICE_TOKEN so one secret never serves two purposes. Rotating it resets the email budgets. Bootstrap credential validation belongs to the auth service when no owner exists.

Auth session TTL and login window must be positive durations; minimum password length and maximum login attempts must be at least one. These startup checks do not require optional owner bootstrap credentials when loading config; EnsureOwner validates credentials when no owner exists.

Provider configuration validates timeout, response-byte cap and breaker limits before startup. The runtime response-byte default lives in `config.yaml`; `DefaultProviderMaxResponseBytes` remains for the programmatic compatibility catalog constructor. Private destination exceptions default to empty; they authorize HTTP and private/loopback connectivity only for exact hostname:port matches. See the [outbound module](../outbound/README.md) for URL, DNS and redirect policy. Allowlist validation never echoes malformed input.

The account-wide login limit uses CORE_AUTH_LOGIN_EMAIL_MAX_ATTEMPTS independently of the email/IP limit. CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS is a separate IP-only budget across emails and must be at least one. Trusted proxy CIDRs must parse successfully. CORE_AUTH_SESSION_TOUCH_INTERVAL controls last_seen_at write frequency. Provider synchronization and breaker cache size are bounded by CORE_PROVIDER_MAX_REMOTE_MODELS and CORE_PROVIDER_MAX_BREAKERS; both must be positive. CORE_PROVIDER_BREAKER_* is the only breaker configuration namespace; unused CORE_BREAKER_* settings are no longer read. Deployment and example validation are covered by deploy_test.go.
