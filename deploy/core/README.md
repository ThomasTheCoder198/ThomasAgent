# Core stack

## Purpose

Compose core, migration, outbox relay, Postgres, Redis, Prometheus, Grafana and VictoriaLogs. Ragx runs from `deploy/ragx`.

## Entry points

- `compose.yaml`, `.env.example`, `prometheus/prometheus.yml`, `grafana/provisioning/`.
- `victorialogs/Dockerfile` adds a health probe to the pinned upstream image.

## Dependencies

- Uses backend-core, Postgres, Redis, Prometheus, Grafana and VictoriaLogs.
- Used by `task up`, `task down` and `task smoke`.

## Run & test

```bash
cp deploy/core/.env.example deploy/core/.env
# Fill the secrets and owner bootstrap credentials in deploy/core/.env.
task up
task smoke
```

## Conventions

Non-secret core defaults live in `backend-core/internal/platform/config/config.yaml` and are embedded in the Go binary. `deploy/core/.env` contains secrets and deployment overrides. Compose passes it directly to `core`, `migrate` and `outbox-relay` with `env_file`. Compose derives only the database URL, Redis URL and local VictoriaLogs endpoint; the service environment takes precedence over values from `env_file`. Within core, process environment overrides a local dotenv file, which overrides `config.yaml`.

Generate `CORE_VAULT_MASTER_KEY` with `openssl rand -base64 32` and keep it stable to preserve access to encrypted provider keys. Generate a `CORE_SERVICE_TOKEN` and a different `CORE_AUTH_LOGIN_LIMITER_KEY`, each of at least 32 characters (for example `openssl rand -hex 32`). Set `CORE_AUTH_OWNER_EMAIL` and a `CORE_AUTH_OWNER_PASSWORD` of at least 12 characters before first startup. Production should set `CORE_ENV=prod` and `CORE_AUTH_COOKIE_SECURE=true` in `.env`. The three core commands currently load the same required config, so they all receive these secrets.

Core exports OTLP logs to VictoriaLogs and internal `/metrics` for Prometheus. A VictoriaLogs outage does not stop core. Grafana waits for Prometheus and VictoriaLogs. Migration is a one-shot gate for the API and relay. The relay has no healthcheck yet. Prometheus and VictoriaLogs bind host loopback only. `VL_RETENTION` and `PROMETHEUS_RETENTION` can override storage retention in Compose. Container log rotation follows Docker daemon defaults after removal of the Compose logging anchor.

Provider, login, Argon, HTTP, retry, stream and relay tuning lives in `config.yaml`; environment variables can override individual values. Trusted proxy headers default to disabled and must be configured together with trusted CIDRs. Provider private network exceptions require exact `host:port` entries. The [config module](../../backend-core/internal/platform/config/README.md) describes validation and precedence. Compose config tests use the example file without exposing local credentials.

## Common failures

- Startup fails when secrets or owner bootstrap credentials are missing or invalid.
- An empty custom `CORE_DATABASE_URL`/`CORE_REDIS_URL` in `.env` uses Compose's service URL; a host-run core needs reachable URLs in its environment.
- An unavailable log backend leaves stdout logs available; check Docker's daemon log rotation settings for long-running deployments.
