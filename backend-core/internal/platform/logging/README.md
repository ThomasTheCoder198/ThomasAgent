# Logging

## Purpose
Owns JSON logs, secret redaction and trace correlation.

## Entry points
- `New`, `ParseLevel`, `RedactValue`

## Dependencies
- Uses: slog and OTEL trace; consumed by core and HTTP boundaries

## Run & test
```bash
cd backend-core
go test ./internal/platform/logging/... -count=1
```

## Conventions
Secret leaf keys and bearer/sk values are redacted in JSON and OTEL, including bound attributes and groups. Usage token counters remain visible. The configured level applies to both sinks. Do not log raw document content or opaque objects containing credentials.

`ParseLevel` owns the supported level names (debug/info/warn/error). Both startup configuration and `New` use it; unknown names return errors instead of silently choosing Info. `New` returns a logger and an error.

The fanout handler attempts every enabled sink and returns their joined errors, including JSON writer failures. An OTEL failure still allows the JSON line to be written. Callers using `slog.Logger` should know its convenience logging methods discard handler errors; call `Handler().Handle` when the write result is needed.

## Common failures
- Missing trace IDs indicate there is no valid span context.
- Invalid log level causes startup to fail; use debug, info, warn, or error.
- Sink write errors can be inspected from the handler return value with `errors.Is`.

Secret ancestor group names also redact descendants for both direct groups and derived loggers.

String-key maps (including typed maps) and nested slices/arrays are recursively copied and redacted before either sink receives them. Byte slices retain their binary encoding; opaque objects are outside this container policy. Input containers are not mutated.
