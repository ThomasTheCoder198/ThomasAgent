# Ragx integration tests

## Purpose
Integration tests against real Postgres, Redis and MinIO through testcontainers.

## Entry points
- `test_migrations.py`: tests migration pairing and up/down behavior.
- `conftest.py`: shared containers and an isolated pool fixture.
- `api_client.py`: scoped App token and ASGI client for API tests.
- `test_files_api.py`: presign, POST to MinIO, confirm, dedupe, download-url and tenant isolation. The helpers in `api_client.py` (`presign`, `post_to_storage`, `confirm`, `upload`) do a real multipart POST to the MinIO container, as a browser does; `api_client` signs for the container's address.
- `test_kb_api.py`: provisioning, tenant isolation, Knowledge Base CRUD and manifest tests.

## Dependencies
- Uses: pytest, testcontainers and Docker.
- Used by: ragx test suite and CI.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration
```

On Windows, set `DOCKER_HOST=npipe:////./pipe/docker_engine` and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock` first.

## Conventions
Images match deploy/ragx; the pool fixture truncates every table before each test.

## Common failures
- Docker not running → start Docker Desktop.
- Rootless Docker message on Windows → set `DOCKER_HOST` as documented.

The session Qdrant fixture uses `qdrant/qdrant:v1.19.1-unprivileged` and a 30-second client timeout; each test clears collections. `api_client` accepts `qdrant_url`, otherwise its Qdrant endpoint is unreachable.
