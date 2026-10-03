# Backend core

## Purpose
Go API and agent runtime for ThomasAgent.

## Entry points
- `cmd/core`: `serve|migrate|relay` command family. Only serve is implemented at this checkpoint; Tasks 7?8 add migrate and relay.
- GET `/healthz`: liveness. GET `/readyz`: injected dependency checks.

## Dependencies
- Uses platform config, apperr, httpx, logging and tracing; see each README in `internal/platform/*`.
- Used by the frontend and service clients.

## Run & test
```bash
cd backend-core
CORE_DATABASE_URL=x CORE_REDIS_URL=y go run ./cmd/core serve
go test ./... -count=1
golangci-lint fmt ./...
golangci-lint run ./...
```

## Conventions
Go 1.27.1 is pinned. CORE_ variables configure startup. Optional CORE_OTLP_ENDPOINT enables OTLP HTTP traces and logs. Readiness adapters arrive in Tasks 7?8; currently the dependency map is empty. All HTTP responses use the shared envelope.

## Common failures
- Missing database/Redis environment values fail config loading.
- An occupied HTTP address prevents startup.
- An unreachable OTLP collector prevents telemetry delivery.
