# UI Message Stream

## Purpose
Encodes run events as AI SDK UI Message Stream v1 chunks and rebuilds the assistant message the browser builds.

## Entry points
- `NewEncoder` and `Encoder.Encode` — produce JSON payloads for SSE data lines.
- `NewMessageBuilder`, `MessageBuilder.Apply`, `Parts`, and `Metadata` — assemble stored assistant messages.

## Dependencies
- Uses: `internal/chat/model` and the AI SDK generated goldens in `testdata`.
- Used by: chat controller and run service.

## Run & test
```bash
go test ./internal/chat/... -count=1
```

## Conventions
Goldens come from `frontend/scripts/capture-ui-stream-goldens.ts`; regenerate them when `ai` is bumped and fix Go, never the goldens.

## Common failures
- A JSONEq diff on a golden frame means the encoder emits a field the SDK does not, or misses one.
