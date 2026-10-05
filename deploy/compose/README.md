# compose

## Purpose
Composes infrastructure, apps, and observability with shared service names and startup gates.

## Entry points
- compose.infra.yaml, compose.observability.yaml, compose.yaml, .env.example

## Dependencies
- Uses services configured in ../compose.
- Used by the deployment compose stack.

## Run & test
```bash
task up
task smoke
```

## Conventions
Configuration is mounted read only; credentials come from compose/.env. Every typed core
setting is forwarded through `x-core-env` to `core`, `migrate` and `outbox-relay`.
`.env.example` lists all runtime defaults; blank database and Redis URLs use compose
service addresses. Copy the example, then fill intentionally blank secrets and bootstrap credentials.

Prometheus, Grafana, and both Langfuse services have real HTTP healthchecks.
Grafana waits for healthy Prometheus; Langfuse web waits for a healthy worker.
Collector, Loki, and `outbox-relay` still need additional internal probe implementations;
their prescribed images lack an HTTP client, and `outbox-relay` has no readiness endpoint.
The migration and bootstrap jobs remain one-shot completed-successfully gates.

## Common failures
- Startup errors: inspect compose logs and validate environment variables.

The shared `x-core-env` passes `CORE_VAULT_MASTER_KEY` to migration, API and outbox
processes. Generate it with `openssl rand -base64 32` and put it in `.env` before
startup. Keep the key stable; changing it prevents decryption of stored secrets.

Compose supplies configuration to M0.2 platform-scoped services. Their data belongs to the [Platform tenant](../../docs/glossary.md) (`default`). M1 business tenancy must derive `tenant_id` from trusted context, never client or model parameters.

The shared `x-core-env` passes CORE_SERVICE_TOKEN, CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD to the migration, API and outbox processes. The example leaves bootstrap credentials empty; supply an owner email and a password of at least 12 characters.
Provider catalog configuration passes through the shared core environment:
`CORE_PROVIDER_HTTP_TIMEOUT` (20s), `CORE_PROVIDER_MAX_RESPONSE_BYTES` (8388608),
and `CORE_PROVIDER_PRIVATE_ALLOWLIST` (empty by default), plus the independent
`CORE_PROVIDER_BREAKER_*` threshold, timeout and half-open settings. These are the only
breaker names; obsolete `CORE_BREAKER_*` names are no longer read. Provider sync and
breaker storage bounds use `CORE_PROVIDER_MAX_REMOTE_MODELS` and `CORE_PROVIDER_MAX_BREAKERS`.
Login rate limits include email/IP, email-global and independent IP-only limits. `CORE_AUTH_LOGIN_IP_MAX_ATTEMPTS` defaults to 100, is validated as at least one, and is forwarded to core, migrate and outbox-relay through `x-core-env`. Email HMAC identity uses the service token. The email-global lockout trade-off is documented in [auth](../../backend-core/internal/auth/README.md).
The client IP defaults to the connection peer. Configure both `CORE_AUTH_TRUSTED_PROXY_HEADER`
and comma-separated `CORE_AUTH_TRUSTED_PROXY_CIDRS` only for known reverse proxies.
Session touch/purge intervals, Argon parameters, HTTP deadlines, retries, stream consumers
and outbox relay settings use the names in `.env.example`.
Only exact `host:port`
entries authorize private/HTTP provider endpoints; redirects remain disabled.
Use `.env.example` for provider circuit-breaker settings. `task smoke` now checks
owner login, CSRF, provider creation and cleanup; it does not change model roles.

Generate `CORE_SERVICE_TOKEN` with `openssl rand -base64 32`; it must have at least 32
characters. Never print local credentials while validating configuration. The config
package's `TestCompose_ExampleConfigurationValid` fills intentionally blank required
secrets with test values and validates the other example settings. `TestCompose_ForwardsEveryCoreSetting`
runs `docker compose config` against the example and checks all typed settings reach
every core process, without printing the resolved credentials.
