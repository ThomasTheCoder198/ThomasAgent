# Logging

## Purpose
Owns JSON logs, secret redaction and trace correlation.

## Entry points
- `New`, `RedactValue`

## Dependencies
- Uses: slog and OTEL trace; consumed by core and HTTP boundaries

## Run & test
```bash
cd backend-core
go test ./internal/platform/logging/... -count=1
```

## Conventions
Secret leaf keys and bearer/sk values are redacted in JSON and OTEL, including bound attributes and groups. Usage token counters remain visible. The configured level applies to both sinks. Do not log raw document content or opaque objects containing credentials.

## Common failures
- Missing trace IDs indicate there is no valid span context.

Secret ancestor group names also redact descendants for both direct groups and derived loggers.
