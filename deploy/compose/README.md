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
Configuration is mounted read only; credentials come from compose/.env.

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
and `CORE_PROVIDER_PRIVATE_ALLOWLIST` (empty by default). Only exact `host:port`
entries authorize private/HTTP provider endpoints; redirects remain disabled.
Use `.env.example` for provider circuit-breaker settings. `task smoke` now checks
owner login, CSRF, provider creation and cleanup; it does not change model roles.
