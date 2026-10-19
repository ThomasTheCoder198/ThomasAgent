# httpx

## Purpose
Owns JSON responses, request IDs and Gin error boundaries.
Structured logger setup is injected by the application. Core has no tracing middleware.

## Entry points

`RecordMetrics(HTTPMetrics)` observes method, route pattern, status and duration after routing, with the request id as exemplar; nil disables it. `NewRouter` callers install it after `LogAccess`. The engine uses `gin.New()` with exact paths and enveloped 404/405 responses; startup sets Gin release mode once. Gin exposes mode as package-global state; core sets it only at startup while request dependencies stay injected.
- `DecodeJSON` requires `application/json`, rejects unknown fields, reads one size-limited JSON value and maps invalid bodies to catalog errors. Missing, malformed or other media types return `UNSUPPORTED_MEDIA_TYPE` (415); malformed JSON and unknown fields return `VALIDATION_FAILED` (400). Requiring JSON and rejecting unknown fields are breaking changes for permissive callers; JSON charset parameters remain supported.
- `ContractRoute` rewrites gin patterns to the contract spelling (`:orgId` and the catch-all `*path` become `{orgId}` and `{path}`) for the access log, metric labels and the `ragx call` route; `HeaderContentTypeOptions`/`ContentTypeOptionsNoSniff` are set on proxied app responses.
- `HeaderContentType`, `MediaTypeJSON`, `HeaderAcceptLanguage`, `HeaderIdempotencyKey` and `ValidIdempotencyKey` are the one definition of the headers the app proxy forwards; `platform/ragx` and `appproxy` reuse them.
- `EventStream` commits SSE headers and serializes flushed data and heartbeat frames. `NewEventStream` requires typed heartbeat and positive per-write timeout settings. Deadlines also bound the initial header flush; write failures close `Done` so producers stop. Request cancellation closes `Done` independently of an in-progress heartbeat write through `context.AfterFunc`. `Close` unregisters this callback.
- `ResponseCommitted` checks Gin's `Written()` state and unwraps nested writers. `Success`, `Fail`, and `Respond` adapt the net/http response primitives for controllers.
- `RespondPage(c, status, data, page, err)` is `Respond` for a cursor-paged list: it adds `meta.page = {cursor, limit}` from `PageInfo` (a nil `Cursor` serializes as `null` and marks the last page). Other responses carry no `page` (`omitempty`).
- `LimitRequestDuration` sets the context deadline for ordinary routes.
- `PathUUID(c, name)` parses a UUID path parameter into a field error named after the parameter.
- `NewRouter` registers assignRequestID → caller middleware → injectErrorLogger → recoverPanics.
- `WriteSuccess` serializes success data and request ID metadata.
- `WriteError` localizes catalog errors and sanitizes server errors.
- `requestid.FromContext` retrieves the shared request ID for callers that need it; logging receives it automatically.

## Dependencies
- Uses: `internal/errors`, platform logging, Gin v1.10.0, and `internal/platform/requestid`.
- Used by: domain controllers and the core router.

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
`ResponseMeta` carries `requestId`, and `page` only for `RespondPage` (spec §18.2; the Conversations list is its first user).
5xx details are never serialized; they are logged once via ErrorLogger.
All 5xx codes and custom messages become the catalog INTERNAL_ERROR response.
404, 405 and recovered panics use the error response. ErrorLogger may be nil.
Routes return named definitions such as `errors.ErrNotFound`; causes and details use their
immutable builder methods. `WriteError` normalizes both named errors and `*AppError` values.
Callers must supply access logging middleware before registering routes.

## Common failures
- Stream arrives all at once: a proxy or middleware is buffering — see spike S8.
- `Send` returns `errors.ErrEventStreamClosed`: the client is gone, stop producing.
- A route changes to a redirect: register the exact Gin path and keep trailing-slash redirects disabled.
- Panic after headers were sent: recovery cannot replace an already committed response.

## Health and observability
`MountHealth` uses `LivenessPath` and `ReadinessPath` with injected `ReadinessDependency` capabilities. See [liveness and readiness](../../../../docs/glossary.md) for route meanings.

`assignRequestID` accepts `X-Request-Id` only when it matches the shared pattern (`contracts/request-id.json`);
otherwise it generates a `req_` id and never logs the rejected value. The id is in context, in the response header,
in `meta.requestId` and in `error.requestId`. `LogAccess` writes one line per request (`http request`) with
`method`, `route` (OpenAPI brace pattern, `unmatched` when none matched), `status`, `duration_ms`, optional
`idempotency_key`, and `request_id_source`/`client_request_id_rejected` when the id was generated. Level follows
the status: 2xx/3xx info, 4xx warn, 5xx error. `RoutePattern` exposes the same low-cardinality route to metrics.

`assignRequestID` uses `requestid.NewContext`; handler, access, and error logs follow the shared [log field contract](../logging/README.md#log-field-contract).

Gin's writer preserves the first committed response status, including implicit commitment by Write or Flush.

## Streaming and API conventions
- A controller decodes with `DecodeJSON`, calls a service, then writes with `Success`, `Fail`, or `Respond`. Services never accept `http.ResponseWriter` or `*http.Request`.
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

`EventStream.Send` rejects CR and LF before writing so a payload cannot introduce extra SSE lines or frames. Event stream flushing unwraps Gin's writer after committing its status, preserving unsupported-flush and `FlushError` failures where the underlying writer exposes them.
