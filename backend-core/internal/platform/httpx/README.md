# httpx

## Purpose
Owns JSON response envelopes, request IDs and chi error boundaries.
Tracing and structured logger setup are injected by the application.

## Entry points
- `NewRouter` registers requestID → caller middleware → errorLogging → recoverer.
- `WriteData` serializes success data and request ID metadata.
- `WriteError` localizes catalog errors and sanitizes server errors.
- `RequestIDFrom` retrieves the request ID for downstream logging.

## Dependencies
- Uses: apperr, chi v5, UUID and OpenTelemetry trace context.
- Used by: API handlers and the application router.

## Run & test
```bash
cd backend-core
go test ./internal/platform/httpx/... -count=1
golangci-lint run ./internal/platform/...
```

## Conventions
Success envelope:
```json
{ "data": { ... }, "meta": { "requestId": "req_...", "page": { "cursor": "...", "limit": 20 } } }
```
Error envelope:
```json
{ "error": { "code": "KB_FILE_NOT_FOUND", "message": "Không tìm thấy tài liệu.", "details": { ... }, "traceId": "4bf9..." } }
```
The current Meta type supplies requestId; pagination is reserved for paginated handlers.
5xx details are never serialized; they are logged once via ErrorLogger.
All 5xx codes and custom messages become the catalog INTERNAL_ERROR response.
404, 405 and recovered panics use the error envelope. ErrorLogger may be nil.
Trace IDs come from active OpenTelemetry span context; absent trace context omits traceId.
Callers must supply tracing and access logging middleware before registering routes.

## Common failures
- Missing traceId: tracing middleware must attach an active span context.
- Middleware registration panic: supply middleware to NewRouter before adding routes.
- Panic after headers were sent: recovery cannot replace an already committed response.

## Health and observability
`MountHealth` registers GET `/healthz` and `/readyz` using the shared envelope. Liveness returns ok; readiness pings injected dependencies. A failed dependency returns 503 with INTERNAL_ERROR publicly and retains PROVIDER_UNAVAILABLE plus dependency names in the boundary log.

`Tracing` wraps handlers with otelhttp; install it before `AccessLog` for trace correlation. `SlogErrorLogger` records structured boundary errors. Access logs include method, path, status, duration and request ID.

Access logging preserves the first final response status, including implicit commitment by Write or Flush; informational responses do not commit the final status.
