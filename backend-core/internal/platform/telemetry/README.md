# Telemetry

## Purpose
Owns core's OTLP log export and its shutdown. Core has no traces and no spans (spec
`2026-10-06-observability-split-design.md` §4.1); requests are correlated by `request_id`.

## Entry points
- `Setup(ctx, serviceName, env, logsEndpoint)` — `cmd/core` at startup; returns the slog handler passed to
  `logging.NewLogger` and a `Shutdown` that flushes pending records.

## Dependencies
- Uses: OpenTelemetry log SDK, `otlploghttp`, `otelslog`.
- Used by: `cmd/core`.

## Run & test
```bash
cd backend-core
go test ./internal/platform/telemetry -count=1
```

## Conventions
`CORE_LOGS_OTLP_ENDPOINT` is the complete logs URL, used as is: no `/v1/logs` is appended. VictoriaLogs
(M0.5.2): `http://victorialogs:9428/insert/opentelemetry/v1/logs`; legacy OTEL Collector: `http://otel-collector:4318/v1/logs`.
Empty disables export; stdout JSON logs continue either way. Resource attributes (`service.name`,
`deployment.environment`) become VictoriaLogs stream fields; never add per-request values there. Shut down after
HTTP stops, with a caller-provided deadline.

## Common failures
- Logs missing in the backend → endpoint lacks the logs path or the backend is down → check
  `CORE_LOGS_OTLP_ENDPOINT`; core keeps serving and stdout still has every line.
