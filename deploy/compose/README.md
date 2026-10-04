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
