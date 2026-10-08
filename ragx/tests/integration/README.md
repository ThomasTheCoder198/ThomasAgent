# Ragx integration tests

## Purpose
Integration tests against real Postgres, Redis and MinIO through testcontainers.

## Entry points
- `test_migrations.py`: tests migration pairing and up/down behavior.
- `conftest.py`: shared containers and an isolated pool fixture.
- `api_client.py`: scoped App token and ASGI client for API tests.
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
