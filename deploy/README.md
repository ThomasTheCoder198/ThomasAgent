# Deployment

## Purpose
Builds service images and runs the local infrastructure, application, and observability stacks.

## Entry points
- `compose/compose.infra.yaml`: Postgres, Redis, Qdrant, and MinIO.
- `compose/compose.observability.yaml`: collector, metrics, logs, Grafana, and Langfuse.
- `compose/compose.yaml`: migrations, core, `outbox-relay`, RAG bootstrap, and API.
- `../scripts/smoke.sh`: checks service health and the core error response.

## Dependencies
- Uses Docker Desktop Linux engine and Docker Compose; builds backend-core and ragx.
- Used by root Taskfile up, down, migrate, and smoke commands.

## Run & test
```bash
# Create only if absent; preserve an existing environment file.
cp -n deploy/compose/.env.example deploy/compose/.env
task up
task smoke
task down
```

## Ports
| Service | Host port |
|---|---|
| Core | 8080 |
| RAG API | 8090 |
| Grafana | 3001 |
| Langfuse | 3002 |
| MinIO API / console | 9000 / 9001 |
| Qdrant | 6333 |
| Postgres / Redis | 5432 / 6379 |
| OTLP HTTP | 4318 |
| Prometheus / Loki | 9090 / 3100 |

## Conventions
Shared response and health terminology is defined in the [glossary](../docs/glossary.md).
Migrations must finish successfully before core and `outbox-relay` start. RAG bootstrap creates
buckets before RAG API and Langfuse worker start. Named volumes preserve data on down.
The collector release uses the published 0.162.0-amd64 tag: this stack requires linux/amd64.

Create a Langfuse account and project at http://localhost:3002 on first boot, then set
LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY in compose/.env and recreate otel-collector.
Placeholder keys keep collector startup valid but exported traces receive authentication errors.
The media URL must be reachable by your browser; internal requests use MinIO's service hostname.

To reset all development data, run the root COMPOSE command from Taskfile with `down -v`;
this permanently removes its named volumes. A fresh Postgres volume creates the Langfuse database.

Prometheus readiness, Grafana health, and Langfuse worker/web health/readiness use HTTP
checks with wget already present in their pinned images. Grafana waits for healthy
Prometheus and Langfuse web waits for a healthy worker. Langfuse checks use service DNS
because the pinned services bind their container hostname rather than loopback.
Collector, Loki, and `outbox-relay` have no in-image HTTP probe client; `outbox-relay` also exposes no
readiness endpoint. Their compose healthchecks remain unresolved pending a probe
implementation. Grafana can only wait for Loki to start until such a probe exists.

## Common failures
- Failed migrate prevents core and `outbox-relay` startup: inspect migrate logs and fix the migration.
- Existing Postgres volume lacks langfuse: create that database or reset development volumes.
- Health failures: inspect compose logs and check occupied host ports and .env credentials.

Langfuse settings follow the [official v4 compose guide](https://langfuse.com/self-hosting/deployment/docker-compose)
and its [source compose](https://github.com/langfuse/langfuse/blob/main/docker-compose.yml).
