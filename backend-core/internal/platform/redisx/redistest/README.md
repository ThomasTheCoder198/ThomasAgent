# Redis test helper

## Purpose
Supplies a disposable Redis client for tests that need real streams, pub/sub or scripts. Each call owns a separate
container. It never touches the development stack.

## Entry points
- `Start(t)` returns a connected `*redis.Client` and registers its cleanup.

## Dependencies
- Uses: go-redis, testify and the testcontainers Redis module.
- Used by: `chat/runstream`, `chat/control` and other core tests that need Redis.

## Run & test
```bash
cd backend-core
go test ./internal/chat/runstream ./internal/chat/control -count=1 -p 1
```
Docker Desktop must run with Linux containers and access to `redis:8.10.2`. On Windows set
`DOCKER_HOST=npipe:////./pipe/docker_engine`, `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock` and
`TESTCONTAINERS_RYUK_DISABLED=true`.

## Conventions
The image tag matches the one `deploy/core` runs, so tests exercise the same Redis commands (`EXPIREAT … NX`
needs Redis 7 or later).

## Common failures
- Container cannot start → Docker Desktop is stopped or the image cannot be pulled → start it, then rerun.
