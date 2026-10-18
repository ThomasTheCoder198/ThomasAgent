# Chat model

## Purpose
Defines chat requests, run events, citation metadata, usage, localized statuses, and public in-stream errors.

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
- A JSONEq diff on a golden frame means the encoder emits a field the SDK does not, or misses one.
