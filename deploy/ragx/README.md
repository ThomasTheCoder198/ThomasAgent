# Ragx stack

## Purpose
Runs ragx with its own Postgres (metadata), Redis (jobs), Qdrant, MinIO and authenticated Phoenix, separate from the core stack (observability spec §8). Phoenix receives ragx traces and stores them in SQLite on `phoenix-data`; UI and OTLP HTTP use port 6006.

## Entry points
- `compose.yaml` starts bootstrap, API and worker processes with Postgres, Redis, Qdrant and MinIO.
- `.env.example` documents required credentials and deployment settings; non-secret runtime defaults live in `ragx/src/thomas_ragx/platform/config.yaml`.

## Dependencies
- Uses: ragx Docker image, Qdrant, MinIO and Phoenix.
- Used by: `task up:ragx` and `task down:ragx`.

## Run & test
```bash
cp -n deploy/ragx/.env.example deploy/ragx/.env
# Fill secrets before startup, then add the public key printed by the next command.
task ragx:dev-key
task up:ragx
task ragx:seed-dev
task smoke
curl -fsS http://localhost:8090/healthz
curl -fsS http://localhost:6006/healthz
# Open http://localhost:6006 and sign in as admin@localhost.
task down:ragx
```

## Conventions
The compose project is `thomas-ragx`; it shares no network with core. `ragx-bootstrap`, `ragx-api` and `ragx-worker` read `.env` directly through `env_file`; Compose only derives topology-specific URLs and shared MinIO/Phoenix credentials. Ragx never calls core. Core calls ragx with App tokens; ragx trusts the keys in `RAG_CORE_PUBLIC_KEYS`. `ragx-postgres` and `ragx-redis` publish no host port. `ragx-bootstrap` runs migrations, buckets and the Qdrant check; `ragx-worker` has a heartbeat-file healthcheck. `RAGX_POSTGRES_PASSWORD` must be hex (it is embedded in `RAG_DATABASE_URL`). Losing `RAG_VAULT_MASTER_KEY` makes stored provider keys unreadable. `task ragx:seed-dev` binds the embedding role to an offline dev model; KBs created before a real role is set keep that model, so set the real embedding role in `/ui` before creating KBs you keep. The admin UI at `http://localhost:8090/ui` is the only browser-facing ragx surface: bootstrap creates its single account once from `RAG_ADMIN_EMAIL` / `RAG_ADMIN_INITIAL_PASSWORD` (≥ 12 characters; later changes to these variables do nothing), and behind HTTPS set `RAG_UI_COOKIE_SECURE=true`. `RAG_OTLP_ENDPOINT` defaults to `http://phoenix:6006`; an explicitly empty value disables export. Ragx has no startup dependency on Phoenix and keeps serving if tracing is unavailable.
Bootstrap explicitly sets an empty `RAG_OTLP_ENDPOINT`: it still logs its work and creates local spans, but does not send startup spans to Phoenix before Phoenix is ready. The API retains the configured Phoenix exporter.

`PHOENIX_SECRET` and `PHOENIX_ADMIN_SECRET` must each have at least 32 characters, a digit and a lowercase letter, and must differ. In dev, `RAG_PHOENIX_API_KEY` equals the admin secret. Production uses a Phoenix system API key instead; remove the admin secret after replacing the ragx key.

`PHOENIX_DEFAULT_ADMIN_INITIAL_PASSWORD` sets the password only when the initial admin account is created. Without an override, Phoenix defaults to `admin`; change it on first login. Later password changes use the UI, not the environment. `RAG_TRACE_CONTENT=false` keeps instrumentor inputs/outputs hidden; enable it only for deliberate debugging/eval. Logs stay as stdout JSON: `docker compose --env-file deploy/ragx/.env -f deploy/ragx/compose.yaml logs ragx-api`.

`QDRANT_HOST_PORT` defaults to 6333 and changes only the published host port. On Windows with reserved ports, use an available value such as 16333 in the local `.env`. Qdrant remains on port 6333 inside the stack.

`PHOENIX_HOST_PORT` defaults to 6006; Windows may require 16006 in the local `.env`. Internal OTLP and healthchecks remain on 6006. Set core's `GRAFANA_PHOENIX_URL` to the browser-accessible Phoenix URL when overriding the host port; smoke reads the local port automatically.

Core-only smoke skips ragx only when its API container is absent. An existing ragx API with a failing health endpoint makes smoke fail, including a stopped or crashed API.

Named volumes preserve data; container log rotation follows Docker daemon defaults. Bootstrap is a one-shot gate; the API image provides its healthcheck.

## Common failures
- Bootstrap fails: inspect MinIO/Qdrant health and compose logs.
- Bootstrap fails on migrations: inspect ragx-postgres health and check `RAGX_POSTGRES_PASSWORD` against the existing volume.
- Smoke returns 401 `APP_TOKEN_INVALID`: check that `RAG_CORE_PUBLIC_KEYS` contains the dev key's kid.
- Published port unavailable: set `QDRANT_HOST_PORT` to an available host port.
- Phoenix port unavailable: set `PHOENIX_HOST_PORT` and align core's `GRAFANA_PHOENIX_URL`.
- Phoenix volume permission error with a nonroot image: initialize only that volume once with `docker run --rm -v thomas-ragx_phoenix-data:/mnt/data alpine chown -R 65532:65532 /mnt/data`. The pinned image currently runs as root and needs no initialization.
- Rejected traces: check the Phoenix auth settings and ragx API key; an export failure does not fail API requests.
