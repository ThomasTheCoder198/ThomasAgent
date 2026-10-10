# Deployment

## Purpose
Runs independent core and ragx stacks for their separate production servers.

## Entry points
- `core/compose.yaml`: core, migrate, outbox-relay, Postgres, Redis, Prometheus, Grafana and VictoriaLogs.
- `ragx/compose.yaml`: ragx bootstrap/API/worker, Postgres, Redis, Qdrant, MinIO and authenticated Phoenix.
- `../scripts/smoke.sh`: health, identity, log correlation and metrics probes.

## Dependencies
- Uses: Docker Desktop with Linux containers and Docker Compose; builds backend-core and ragx.
- Used by: root Taskfile deployment and migration commands.

## Run & test
```bash
cp -n deploy/core/.env.example deploy/core/.env
cp -n deploy/ragx/.env.example deploy/ragx/.env
# Fill credentials; keep an existing vault key stable.
task up
task migrate
task smoke
task up:ragx
task down:ragx
task down
```

## Conventions
Core sends OTLP logs directly to VictoriaLogs and exposes metrics on port 9464 inside its compose network. Core and ragx have independent compose projects and networks. Retention can be overridden from each stack's `.env`; container log rotation follows Docker daemon defaults.

| Service | Host port |
| --- | --- |
| Core / Grafana | 8080 / 3001 |
| Prometheus / VictoriaLogs | 127.0.0.1:9090 (`PROMETHEUS_HOST_PORT`) / 127.0.0.1:9428 |
| Postgres / Redis | 5432 / 6379 (`REDIS_HOST_PORT`) |
| Ragx API | 8090 |
| Phoenix UI / OTLP HTTP | 6006 (`PHOENIX_HOST_PORT`) |
| MinIO API / console | 9000 / 9001 |
| Qdrant | 6333 (`QDRANT_HOST_PORT`) |

Every persistent service has a healthcheck except outbox-relay, which currently exposes only its internal metrics listener. Migration and bootstrap are one-shot gates. `task down` preserves named volumes; core reuses the existing `thomas` Postgres and Redis volumes. The new `thomas-ragx` project initializes separate Qdrant/MinIO volumes, and the old volumes remain preserved.

## Common failures
- Logs missing: check `CORE_LOGS_OTLP_ENDPOINT` and VictoriaLogs health.
- Prometheus target down: check core health and its configured metrics address.
- Windows rejects ports 6379/6333/9090 (reserved ranges: `netsh interface ipv4 show excludedportrange protocol=tcp`): choose available `REDIS_HOST_PORT`/`QDRANT_HOST_PORT`/`PROMETHEUS_HOST_PORT` values in the local environment files; container ports stay unchanged.
