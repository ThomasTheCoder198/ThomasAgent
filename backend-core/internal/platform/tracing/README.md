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
An empty endpoint disables exporters. W3C trace context and baggage propagation are registered. Pass the OTLP HTTP base endpoint; traces and logs paths are appended. Shut providers down after HTTP stops.

## Common failures
- Collector connectivity failures prevent telemetry delivery; verify CORE_OTLP_ENDPOINT. Task 10 smoke validates export end to end.
