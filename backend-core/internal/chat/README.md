# Chat

## Purpose
The agent chat domain of core: the run loop that answers one user message with the Agent's Knowledge Base tools, and
everything that moves its events to the browser. It does not own conversations, messages or run rows (those are
`internal/conversation`, reached through `ChatStore`), models or providers (`internal/llmprovider`,
`internal/platform/genkitx`), or ragx (`internal/platform/ragx`).

## Entry points
| Folder | Holds | Runs in |
|---|---|---|
| `model/` | `ChatRequest`, the `Event` types a run emits, `CitationLocator`, `RunUsage`, localized status labels, `ErrorEvent` | both |
| `service/` | `RunService`: the run loop, the `worker.Executor` of `core worker` | worker |
| `worker/` | `Handler`: claims a run under a lease, runs the executor, writes the terminal event | worker |
| `runstream/` | per-run Redis Stream: `Writer` (worker), `Reader.Follow` and `RelayEnd` (API) | both |
| `control/` | `Bus`: Stop (and later Steer) from the API to the worker | both |
| `uistream/` | AI SDK UI Message Stream v1 encoder and the stored-message builder | API relay, worker |

Controller, dto and steer folders arrive with the chat endpoint (2b-5) and Steer (2b-6).

## Dependencies
- Uses: `internal/conversation` (`ChatStore`), `internal/agent`, `internal/llmprovider`, `internal/platform/*`.
- Used by: `cmd/core` (`core serve` for the relay, `core worker` for the run).

## Run & test
```bash
cd backend-core
go test ./internal/chat/... -count=1
```
Folders that need Postgres or Redis have their own commands in their README.

## Conventions
- Events are the only contract between the run and the stream: the run emits `model.Event` values, and the worker,
  relay and encoder decide what to do with them.
- A run's end is stored before its terminal event is written (amendment W4).

## Common failures
- A chat answer never starts → the job did not reach the worker → check `core worker` readiness and the `chat.runs` stream.
- The stored message differs from what the browser showed → the builder and the encoder disagree → fix them, never the goldens.
