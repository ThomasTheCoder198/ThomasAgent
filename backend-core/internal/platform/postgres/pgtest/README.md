# Postgres test helper

## Purpose
Supplies disposable Postgres pools with all embedded migrations applied for
integration tests. Each call owns a separate container and database.

## Entry points
- `Start(t)` returns a verified `*pgxpool.Pool` and registers its cleanup.

## Dependencies
- Uses: platform Postgres, embedded migrations, pgxpool, testify, and testcontainers.
- Used by: core module integration tests needing the current schema.

## Run & test
```bash
cd backend-core
go test ./internal/platform/postgres/... -count=1 -p 2
```
Docker Desktop must run with Linux containers and access to postgres:18.6-alpine.
On Windows, set process variables `DOCKER_HOST=npipe:////./pipe/docker_engine`
and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock` when Docker
Desktop is misidentified as a containerized Docker host. Keep Ryuk enabled.

## Conventions
Tests call `Start(t)` instead of connecting to the live development stack.
The returned pool closes before the container is terminated by test cleanup.
The helper applies all pending migrations and does not seed business data.

## Common failures
- Container cannot start: check Docker Desktop and image access.
- Migration fails: inspect the unapplied SQL; the helper fails the test.
- Pool cannot open: check the container's readiness and Docker port forwarding.
