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

Non-secret core defaults live in `backend-core/internal/platform/config/config.yaml` and are embedded in the Go binary. `deploy/core/.env` contains secrets and deployment overrides. Compose passes it directly to `core`, `migrate` and `core-worker` with `env_file`. Compose derives only the database URL, Redis URL and local VictoriaLogs endpoint; the service environment takes precedence over values from `env_file`. Within core, process environment overrides a local dotenv file, which overrides `config.yaml`.

Generate `CORE_VAULT_MASTER_KEY` with `openssl rand -base64 32` and keep it stable to preserve access to encrypted provider keys. Generate a `CORE_AUTH_LOGIN_LIMITER_KEY` of at least 32 characters (for example `openssl rand -hex 32`). Set `CORE_AUTH_OWNER_EMAIL` and a `CORE_AUTH_OWNER_PASSWORD` of at least 12 characters before first startup. Production should set `CORE_ENV=prod` and `CORE_AUTH_COOKIE_SECURE=true` in `.env`. The three core commands currently load the same required config, so they all receive these secrets.

Core exports OTLP logs to VictoriaLogs and internal `/metrics` for Prometheus. A VictoriaLogs outage does not stop core. Grafana waits for Prometheus and VictoriaLogs. Migration is a one-shot gate for the API and relay. The relay has no healthcheck yet. Prometheus and VictoriaLogs bind host loopback only. `VL_RETENTION` and `PROMETHEUS_RETENTION` can override storage retention in Compose. Container log rotation follows Docker daemon defaults after removal of the Compose logging anchor.

Provider, login, Argon, HTTP, retry, stream and relay tuning lives in `config.yaml`; environment variables can override individual values. Trusted proxy headers default to disabled and must be configured together with trusted CIDRs. Provider private network exceptions require exact `host:port` entries. The [config module](../../backend-core/internal/platform/config/README.md) describes validation and precedence. Compose config tests use the example file without exposing local credentials.

## Common failures

- Startup fails when secrets or owner bootstrap credentials are missing or invalid.
- An empty custom `CORE_DATABASE_URL`/`CORE_REDIS_URL` in `.env` uses Compose's service URL; a host-run core needs reachable URLs in its environment.
- An unavailable log backend leaves stdout logs available; check Docker's daemon log rotation settings for long-running deployments.

## Startup order and the App signing key

`core serve` refuses to start without an App signing key, so the core stack mounts one:

1. `task ragx:dev-key` writes `deploy/ragx/dev-app-signing-key.pem` and prints `RAG_CORE_PUBLIC_KEYS`.
2. Put the printed `RAG_CORE_PUBLIC_KEYS` in `deploy/ragx/.env`.
3. `task up:ragx`, then `task ragx:seed-dev`, then `task up`.

If the key file is missing, Docker mounts a directory in its place and core exits with `ErrAppSigningKeyInvalid`.
On Linux the distroless `nonroot` user (uid 65532) must be able to read the dev key: run
`chmod 0644 deploy/ragx/dev-app-signing-key.pem` (dev only; never relax a production key).
`CORE_RAGX_URL` defaults to `http://host.docker.internal:8090`; `CORE_APP_SIGNING_KEY_HOST_FILE` overrides the mounted file.

`core-worker` owns the outbox relay, periodic cleanup and the `chat.runs` consumer that executes chat runs (several replicas may run; each run executes under a lease, at most once). Its readiness also needs the consumer group to exist. Its internal `CORE_WORKER_ADDR` endpoint serves health, readiness and metrics; Prometheus scrapes it on port 9465. The chat run loop calls ragx tools, so `core-worker` gets the same `CORE_RAGX_URL` and app signing key mount as `core`.
