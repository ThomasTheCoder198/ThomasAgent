# Genkit test fixtures

## Purpose
Scripted OpenAI-compatible and System One HTTP servers for core tests.

## Entry points
- `NewOpenAIServer`, `Turn`, `ToolCall`, `Request` script chat calls and inspect requests.
- `NewDecisionsServer` scripts decision answers and failures.

## Dependencies
- Uses: Go `httptest`.
- Used by: `genkitx` and chat tests.

## Run & test
```bash
cd backend-core && go test ./internal/platform/genkitx/...
```

## Conventions
One turn or answer is consumed per successful request. An empty queue answers 500; `Hold` returns on client cancellation.

## Common failures
- Unexpected 500: push enough turns or answers before the call.
