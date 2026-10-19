# Chat control

## Purpose
Delivers Stop to the worker that is running a chat run, on any replica (amendment W6). A stop is a Redis key plus a
publish: the key survives a message nobody was listening for, the publish wakes a listener at once. 2b-6 adds Steer
to the same subscription.

## Entry points
- `Bus.Stop(ctx, runID, until)` (`bus.go`): `SET run-stop:{runId} 1 EXAT until`, then `PUBLISH run-control:{runId}
  stop`. `until` is `deadline_at + finish timeout`. Idempotent; a time in the past is accepted (the key is gone at
  once).
- `Bus.IsStopped(ctx, runID)`: whether the key exists.
- `Bus.Subscribe(ctx, runID)` returns a `Subscription`: `Stopped()` fires once, on a published stop or when a key
  check finds the key; `Check(ctx)` reads the key (the executor calls it between steps and before each tool batch);
  `Close()` releases the Redis subscription.

## Dependencies
- Uses: go-redis, named catalog errors.
- Used by: `chat/worker` (subscribes), `cmd/core` (wiring), the stop endpoint of 2b-5 (`Stop`).

## Run & test
```bash
cd backend-core
go test ./internal/chat/control -count=1 -p 1      # Redis testcontainer
```

## Conventions
- **Subscribe first, then check the key.** `Subscribe` waits for Redis to confirm the subscription before it reads the
  key, so a stop sent before it is found by the key and one sent after by the channel; none falls between.
- Keys and the channel are per run id, so a stop never reaches a later run.
- Always `Close` the subscription (the worker does it when the run ends); an open one holds a Redis connection.

## Common failures
- Stop has no effect → the run's worker could not subscribe (the run then ends `RUN_INTERRUPTED`) or the run already
  ended → check the worker log for `chat run cannot start` with the run's `request_id`.
