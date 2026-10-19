# UI Message Stream

## Purpose
Encodes run events as AI SDK UI Message Stream v1 chunks and rebuilds the assistant message the browser builds. It does not write SSE (`httpx.NewEventStream` does) and does not localize `errorText`: that is the catalog code and the web words it.

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
- A JSONEq diff on a golden frame → the encoder emits a field the SDK does not, or misses one → fix the encoder or builder, never the golden.
- `Encode` returns `ErrUIStreamUnknownToolCall` → a tool output arrived for a call that never had an input event → fix the emitter's event order.
