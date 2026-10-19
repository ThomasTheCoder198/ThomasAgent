# Chat worker

## Purpose
The `chat.runs` consumer handler of `core worker` (amendment W5). It claims a run under a lease so a run executes at
most once, runs the `Executor`, relays its events into the run stream and writes the terminal event after the run's
end is stored. It does not implement the run loop (2b-4 implements `Executor`) and never retries a run: a failed run
is ended, not re-executed.

## Entry points
- `Handler.Handle` (`handler.go`): a `jobs.Handler`. Decodes the `ChatRunJob` payload, puts the job's user and the
  `traceparent` into ctx, then `ClaimRun`:
  - `Claimed`: subscribe to the control channel first and check the stop key, heartbeat every lease/3, run the
    executor under the run deadline, then write its terminal event;
  - `HeldElsewhere`: an initial duplicate is acked and dropped; a redelivery stays pending until the old lease lapses, so a failed settlement remains recoverable. `NotRunning`: ack and drop;
  - `Expired`: `FailRun(RUN_INTERRUPTED, onlyIfExpired)` and, if the row changed, the terminal error;
  - `QueuedPastDeadline`: `FailRun(RUN_DEADLINE_EXCEEDED)` and the terminal error.
- `Executor`, `ExecuteInput`, `CauseCode` (`executor.go`): the contract 2b-4 implements.
- `PlaceholderExecutor` (`placeholder_executor.go`): TEMPORARY, replaced by 2b-4; ends every run `failed RUN_INTERRUPTED` and logs one warning per run; no run reaches it in production before the chat endpoint (2b-5) exists.

## Dependencies
- Uses: `chat/control` (stop subscription), `chat/runstream` (event codec, writer), `chat/model`,
  `conversation/model`, `auth/middleware` (user in ctx), `platform/jobs` (`Message`), `platform/config`, OpenTelemetry
  propagation.
- Used by: `cmd/core` (`core worker` registers it on the `chat.runs` consumer).

## Run & test
```bash
cd backend-core
go test ./tests/integration -run "TestChatWorker_|TestChatRequestID_" -p 1 -count=1   # Postgres + Redis testcontainers
go test ./cmd/core -run "TestWorker_" -p 1 -count=1
```
On Windows set `DOCKER_HOST=npipe:////./pipe/docker_engine`, `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=//var/run/docker.sock`
and `TESTCONTAINERS_RYUK_DISABLED=true`.

## Conventions
- **Only a settled outcome is acked.** If an executor errors or panics and the safety-net `FailRun` fails, `Handle`
  returns that error so the job stays pending. A redelivery while its old lease is live also stays pending; after the
  lease lapses, the redelivery settles it as `RUN_INTERRUPTED` without executing the run again. A persistent database
  failure follows the consumer's retry and dead-letter policy.
- **The executor owns the run's writes** (`RecordStep(lease, …)`, the fenced `FinishRun(lease, …)`) and returns the
  terminal event (`Finish` or `Error`). A lost lease is `errors.ErrRunLeaseLost`; the handler then writes nothing.
- **Run context.** It ends at the run deadline, when a heartbeat is refused or fails, or when the stream cap
  (`CORE_CHAT_RUN_STREAM_MAX_EVENTS`, `_MAX_BYTES`) is exceeded; `context.Cause` tells which, and `CauseCode` gives the
  `error_code` to finish a failed run with. A user Stop does not cancel it: it arrives through `ExecuteInput.Control`.
- `Emit` keeps working after the run context is cancelled, so a partial answer can still be closed; it rejects
  terminal events, which only the handler writes.
- If `Emit` exceeds a stream cap and the executor returns a success terminal without settling the row, the handler
  rejects that terminal and fails the still-running row with `RUN_OUTPUT_TOO_LARGE`. The handler cannot undo an
  executor's already-committed `completed` row. The 2b-4 executor must check the cap cause before `FinishRun` and
  never commit completed after an `Emit` error (ruling F2bW2-6).
- A panic or an error that leaves the run unfinished fails it once (`FailRun`) and writes one terminal error after a
  successful database transition.
- Logs: `chat run claimed|finished|interrupted|dropped`, each with `run_id`; `finished` and `interrupted` carry
  `duration_ms`. `request_id` comes from the job's ctx, so it matches the API's logs for the same chat request.

## Common failures
- `chat run interrupted` with `reason=lease_lost` → a heartbeat was refused (the lease expired or another executor
  took over) → whoever holds the row writes the terminal event; check that worker's log for the same `run_id`.
- A run stuck `running` long after its deadline → no worker consumed the job → check `core worker` readiness
  (`/readyz` fails without the `chat.runs` group) and the `chat.runs` pending list.
