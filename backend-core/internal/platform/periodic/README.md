# Periodic jobs

## Purpose
Runs scheduled core jobs (session purge, conversation retention) inside the worker process. It owns the ticker, the Redis job lock, the per-run request id and deadline, and the outcome log and metric. It does not own the work itself: each `Job.Run` is supplied by the caller and must be idempotent.

## Entry points
- `Runner.Run(ctx, jobs)`: called by `core worker`. Validates every job, ticks each one, and returns once ctx is cancelled and the runs already in flight have finished.
- `Job{Name, Interval, Timeout, Run}`: `Name`, `Run`, a positive `Interval` and a positive `Timeout` are required; otherwise `Run` returns `errors.ErrPeriodicJobInvalid` before starting anything. A zero interval no longer disables a job: do not register a job to turn it off.

## Dependencies
- Uses: go-redis (lock `lock:job:<name>`), `requestid`, `logging`, `metrics` outcome names, slog.
- Used by: `cmd/core/worker.go` (session and conversation purge jobs).
- Interfaces: `JobRecorder.ObserveJob(stream, outcome)` is optional; outcomes are recorded under `periodic:<job name>`.

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/platform/periodic -count=1 -p 1
```
Docker Desktop must run for the Redis container.

## Conventions
- The first run of a job happens one `Interval` after `Run` starts, not at start-up.
- Every run gets a fresh `job_` request id and a context that ends at `Timeout`. Shutdown stops new runs but does not cancel a run that is in flight, so a purge is never cut off; `Timeout` still bounds it. The worker's drain timeout bounds how long the process waits.
- The lock is best effort: a run past its timeout lets another worker start the job, so jobs must be idempotent. Release is a Lua compare-and-delete with its own short timeout, so a late finisher never deletes a newer run's lock.
- A panic in `Job.Run` is recovered, logged once as a failed run, counted as an error and the lock is released.
- Log fields: `job`, `count`, `duration_ms`, `request_id`, `error` (failure only). Failed runs log exactly one error line. Metric: `jobs_processed_total{stream="periodic:<name>", outcome}`; there is no duration histogram yet (logs only).

## Common failures
- `periodic job needs a name, a run function, and a positive interval and timeout` → a job was registered with a zero or negative value → fix the config; config validation should reject it first.
- `periodic lock failed` (error log + `error` outcome) → Redis unreachable → the job is skipped until the next tick.
- `periodic lock busy` (debug log) → another worker holds the lock → expected with several workers.
- `periodic lock release failed` (warn) → Redis hiccup after the run → the lock expires on its own after `Timeout`.
