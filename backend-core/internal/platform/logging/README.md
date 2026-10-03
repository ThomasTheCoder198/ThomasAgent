# Logging

## Purpose
Owns JSON logs, secret redaction and trace correlation.

## Entry points
- `NewLogger(writer, level, service, environment, otelHandler)`, `ParseLevel`, `RedactValue`
- `ContextWithRequestID` and `RequestIDFromContext` share request correlation with HTTP boundaries and application callers.

## Dependencies
- Uses: slog and OTEL trace; consumed by core and HTTP boundaries

## Run & test
```bash
cd backend-core
go test ./internal/platform/logging/... -count=1
```

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
Secret leaf keys and bearer/sk values are redacted in JSON and OTEL, including bound attributes and groups. Usage token counters remain visible. The configured level applies to both sinks. Do not log raw document content or opaque objects containing credentials.

`ParseLevel` owns the supported level names (debug/info/warn/error). Both startup configuration and `NewLogger` use it; unknown names return errors instead of silently choosing Info. `NewLogger` returns a logger and an error.

The multiHandler attempts every enabled sink and returns their joined errors, including JSON writer failures. An OTEL failure still allows the JSON line to be written. Callers using `slog.Logger` should know its convenience logging methods discard handler errors; call `Handler().Handle` when the write result is needed.

### Log field contract

JSON records always include `ts`, `level`, `msg`, `service`, and `env`. `ts` replaces slog's top-level `time` field and uses UTC RFC3339Nano; nested application fields named `time` retain their names and values. Logger construction receives the service and environment from typed config.

A valid active span adds `trace_id` and `span_id`. A request ID stored with `ContextWithRequestID` adds `request_id`; absent context omits these correlation fields. The request middleware preserves a supplied `X-Request-Id` or generates one and stores the same value in context and the response header. Handler, access, and boundary error logs receive it automatically; callers do not repeat it as an explicit attribute.

OTLP records use their native timestamp, severity, and body fields for `ts`, `level`, and `msg`. They receive the same service, environment, trace, span, and request attributes as JSON records. Both outputs apply the configured level and redaction policy.

## Common failures
- Missing trace IDs indicate there is no valid span context.
- Invalid log level causes startup to fail; use debug, info, warn, or error.
- Sink write errors can be inspected from the handler return value with `errors.Is`.

Secret ancestor group names also redact descendants for both direct groups and derived loggers.

String-key maps (including typed maps) and nested slices/arrays are recursively copied and redacted before either sink receives them. Byte slices retain their binary encoding; opaque objects are outside this container policy. Input containers are not mutated.
