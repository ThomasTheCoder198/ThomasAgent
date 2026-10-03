# Backend core

## Purpose
Go API and agent runtime for ThomasAgent.

## Entry points
- `cmd/core`: `serve|migrate|relay` command family. Implements serve, migrate, relay, and healthcheck.
- GET `/healthz`: liveness. GET `/readyz`: injected dependency checks.

## Dependencies
- Uses platform config, apperr, httpx, logging and tracing; see each README in `internal/platform/*`.
- Used by the frontend and service clients.

## Run & test
Local Go commands load `backend-core/.env` when run from this folder. Copy
`.env.example` for a new workstation; process variables override file values.
The local `.env` is ignored by Git and excluded from Docker builds. Compose
continues to use `deploy/compose/.env` and explicit container variables.

`schema.sql` is the complete desired database schema. After changing it, run
`task migration:diff NAME=add_feature` from the repository root. This compares
the existing migration history with the desired schema on disposable Postgres,
validates the generated Up/Down, and writes `migrations/YYYYMMDDHHMMSS_add_feature.sql`.
It does not apply schema changes to the application's database. Review the SQL,
then use `task migrate` to apply it. An unchanged schema produces no migration.

Go errors are declared in generated `internal/errors/errors.go`: use named
values such as `errors.ErrNotFound` and `.WithCause(err)`. Add codes/messages in
`contracts/errors.yaml` and run `task gen`; old `platform/apperr` imports remain
compatible. The core executable's responsibilities are split under `cmd/core`.

```bash
cd backend-core
# Set CORE_DATABASE_URL and CORE_REDIS_URL to reachable services.
go run ./cmd/core migrate up
go run ./cmd/core serve
go run ./cmd/core relay
go run ./cmd/core healthcheck
go test ./... -count=1
golangci-lint fmt ./...
golangci-lint run ./...
```

## Conventions
Go 1.27.1 is pinned. CORE_ variables configure startup. Optional CORE_OTLP_ENDPOINT enables OTLP HTTP traces and logs. Serve opens and pings Postgres and Redis before listening; readiness checks both dependencies. All HTTP responses use the shared envelope.

## Common failures
- Missing database/Redis environment values fail config loading.
- An occupied HTTP address prevents startup.
- An unreachable OTLP collector prevents telemetry delivery.
