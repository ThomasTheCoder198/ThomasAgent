# Chat model

## Purpose
Defines chat requests, run events, citation metadata, usage, localized statuses, and public in-stream errors. It does not encode events for the wire (`uistream`) or store anything.

## Entry points
- `Event` — contract emitted by the run service and consumed by the stream encoder.
- `ChatRequest` — input assembled by the chat controller.

## Dependencies
- Uses: shared error catalog and UUIDs.
- Used by: chat run service and `internal/chat/uistream`.

## Run & test
```bash
go test ./internal/chat/... -count=1
```

## Conventions
Goldens come from `frontend/scripts/capture-ui-stream-goldens.ts`; regenerate them when `ai` is bumped and fix Go, never the goldens.

## Common failures
- A JSONEq diff on a golden frame → the encoder emits a field the SDK does not, or misses one → fix the encoder or builder, never the golden.
- A 5xx code reaches the client as `INTERNAL_ERROR` → it is not in `publicServerError` → add it there only if it is safe to show and says what to do next.
