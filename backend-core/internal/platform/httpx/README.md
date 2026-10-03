# httpx

## Purpose
Owns JSON responses, request IDs and chi error boundaries.
TraceRequests and structured logger setup are injected by the application.

## Entry points
- `NewRouter` registers assignRequestID → caller middleware → injectErrorLogger → recoverPanics.
- `WriteSuccess` serializes success data and request ID metadata.
- `WriteError` localizes catalog errors and sanitizes server errors.
- `logging.RequestIDFromContext` retrieves the shared request ID for callers that need it; logging receives it automatically.

## Dependencies
- Uses: `internal/errors`, platform logging, chi v5, UUID and OpenTelemetry trace context.
- Used by: API handlers and the application router.

## Run & test
```bash
cd backend-core
go test ./internal/platform/httpx/... -count=1
golangci-lint fmt ./...
golangci-lint run ./internal/platform/...
```

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
Response terminology and JSON shape: [glossary](../../../../docs/glossary.md).
The current ResponseMeta type supplies requestId; pagination is reserved for paginated handlers.
5xx details are never serialized; they are logged once via ErrorLogger.
All 5xx codes and custom messages become the catalog INTERNAL_ERROR response.
404, 405 and recovered panics use the error response. ErrorLogger may be nil.
Routes return named definitions such as `errors.ErrNotFound`; causes and details use their
immutable builder methods. `WriteError` normalizes both named errors and `*AppError` values.
Trace IDs come from active OpenTelemetry span context; absent trace context omits traceId.
Callers must supply tracing and access logging middleware before registering routes.

## Common failures
- Missing traceId: tracing middleware must attach an active span context.
- Middleware registration panic: supply middleware to NewRouter before adding routes.
- Panic after headers were sent: recovery cannot replace an already committed response.

## Health and observability
`MountHealth` uses `LivenessPath` and `ReadinessPath` with injected `DependencyPinger` capabilities. See [liveness and readiness](../../../../docs/glossary.md) for route meanings.

`TraceRequests` wraps handlers with otelhttp; install it before `LogAccess` for trace correlation. `NewSlogErrorLogger` records structured boundary errors. Access logs include method, path, status, duration and request ID.

`assignRequestID` uses `logging.ContextWithRequestID`; handler, access, and error logs follow the shared [log field contract](../logging/README.md#log-field-contract).

Access logging preserves the first final response status, including implicit commitment by Write or Flush; informational responses do not commit the final status.
