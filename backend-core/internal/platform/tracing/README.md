# Tracing

## Purpose
Owns OTLP trace and log provider lifecycle.

## Entry points
- `Setup`, `Providers.Shutdown`

## Dependencies
- Uses: OTEL SDK and otelslog; consumed by core

## Run & test
```bash
cd backend-core
go test ./... -count=1
```

## Conventions
An empty endpoint disables exporters. W3C trace context and baggage propagation are registered. Pass the OTLP HTTP base endpoint; trailing slashes are normalized before the named traces and logs paths are appended. Shut providers down after HTTP stops with a caller-provided deadline.

## Common failures
- Collector connectivity failures prevent telemetry delivery; verify CORE_OTLP_ENDPOINT. Task 10 smoke validates export end to end.
