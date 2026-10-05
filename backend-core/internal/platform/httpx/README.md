# httpx

## Purpose
Owns JSON responses, request IDs and chi error boundaries.
TraceRequests and structured logger setup are injected by the application.

## Entry points
- `DecodeJSON` requires `application/json`, rejects unknown fields, reads one size-limited JSON value and maps invalid bodies to catalog errors. Missing, malformed or other media types return `UNSUPPORTED_MEDIA_TYPE` (415); malformed JSON and unknown fields return `VALIDATION_FAILED` (400). Requiring JSON and rejecting unknown fields are breaking changes for permissive callers; JSON charset parameters remain supported.
- `EventStream` commits SSE headers and serializes flushed data and heartbeat frames. `NewEventStream` requires typed heartbeat and positive per-write timeout settings. Deadlines also bound the initial header flush; write failures close `Done` so producers stop. Request cancellation closes `Done` independently of an in-progress heartbeat write through `context.AfterFunc`. `Close` unregisters this callback.
- `ResponseCommitted` checks whether the status line has been sent.
- `LimitRequestDuration` sets the context deadline for ordinary routes.
- `NewRouter` registers assignRequestID → caller middleware → injectErrorLogger → trackCommit → recoverPanics.
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
- Stream arrives all at once: a proxy or middleware is buffering — see spike S8.
- `Send` returns `ErrEventStreamClosed`: the client is gone, stop producing.
- Missing traceId: tracing middleware must attach an active span context.
- Middleware registration panic: supply middleware to NewRouter before adding routes.
- Panic after headers were sent: recovery cannot replace an already committed response.

## Health and observability
`MountHealth` uses `LivenessPath` and `ReadinessPath` with injected `DependencyPinger` capabilities. See [liveness and readiness](../../../../docs/glossary.md) for route meanings.

`TraceRequests` wraps handlers with otelhttp; install it before `LogAccess` for trace correlation. `NewSlogErrorLogger` records structured boundary errors. Access logs include method, path, status, duration and request ID.

`assignRequestID` uses `logging.ContextWithRequestID`; handler, access, and error logs follow the shared [log field contract](../logging/README.md#log-field-contract).

Access logging preserves the first final response status, including implicit commitment by Write or Flush; informational responses do not commit the final status.

## Streaming and API conventions
- A handler decodes with `DecodeJSON`, calls a service, then writes with `WriteSuccess` or `WriteError`. Services never accept `http.ResponseWriter` or `*http.Request`.
- A streaming service returns a channel (or `iter.Seq`) of events; the handler encodes each event and calls `EventStream.Send`. The UI Message Stream encoder belongs to M2.
- Once a stream starts, the status line is sent. Errors become an in-stream `error` part written by the handler (M2). `WriteError` and panic recovery only log; recovery aborts the connection. They never append JSON.
- Ordinary routes use `LimitRequestDuration`; stream routes are mounted in a group without it. The stream lifetime is the agent runtime run deadline (spec §7.1, five minutes).
- CORS is intentionally absent: the browser talks to its own origin and Next.js rewrites `/api/v1/*` to core (M0.3). Revisit only if spike S8 finds the rewrite cannot carry streams.
- Handlers must `defer stream.Close()` to stop heartbeats before returning.

Writers that expose flushing but hide write deadlines (including wrapped `http.ErrNotSupported`)
can stream without deadlines. Production middleware should expose `Unwrap` or forward
`SetWriteDeadline` so slow clients retain the configured network write bound. Supported
writers retain deadlines for both the initial header flush and every data/heartbeat frame;
other deadline errors still fail construction or stop the stream.

`EventStream.Send` rejects CR and LF before writing so a payload cannot introduce extra SSE lines or frames. Recorder flushing forwards through `ResponseController`, preserving unsupported-flush and `FlushError` failures where the underlying writer exposes them. The installed otelhttp legacy `Flush` interface cannot transmit flush errors it discards.
