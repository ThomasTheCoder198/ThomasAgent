# Jobs

## Purpose
Owns transactional outbox enqueueing, Redis stream publishing, pending-message
recovery, acknowledgement, and dead-letter delivery.

## Entry points
- `EnqueueOutbox`: call inside the transaction that writes the business change.
- `OutboxRelay.Run` / `PublishBatch`: `core worker` publishes committed outbox rows.
- `Consumer.EnsureGroup`, `Run`, `PollOnce`: worker entry points.
- `Handler`: application processing; its context carries the originating `request_id` and tenant.

## Dependencies
- Logger construction returns an error for unknown levels; callers must handle it before injecting a logger.
- Uses: pgx/Postgres, go-redis/Redis Streams, `internal/errors`, `requestid` context, slog.
- Used by: core worker and future domain workers.
- Configure batch/poll limits from `config.OutboxRelayConfig`; consumer limits from
  `config.ConsumerConfig`. Callers provide clients and a structured logger.

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/platform/jobs/... -count=1 -p 2
go run ./cmd/core worker
```
Docker Desktop must run for the pinned Postgres/Redis integration containers.

## Conventions

`OutboxRelay.Metrics` receives backlog (`outbox_pending`, `outbox_relay_lag_seconds`) after every batch. `Consumer.Metrics` receives each job outcome (`success`, `retry`, `dead_letter`). Both recorders are optional.
Call `Consumer.EnsureGroup` before processing jobs. When the recorder also implements
`JobStreamRegistrar`, group initialization registers the stream's zero DLQ counter, including when
the group already exists after a worker restart. This gives Prometheus a baseline for the first failure
without resetting counters or requiring observation-only recorders to implement registration.
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
Delivery is at-least-once. A crash after XADD and before transaction COMMIT can
publish the same outbox row again. Handlers must deduplicate by `outbox_id` and
make their business side effects idempotent. Handlers receive the original `outbox_id` directly in `Message.OutboxID`
before applying side effects. Entries created outside the outbox may omit this
field; domain handlers must define an appropriate idempotency key for those jobs.
Enqueue requires trusted tenant context and writes `tenant_id` explicitly. The
platform relay is infrastructure dispatch: it intentionally scans committed rows
across tenants with FOR UPDATE SKIP LOCKED. Publication updates remain scoped by
the row tenant. Redis entries and dead letters preserve `tenant_id`; consumers
replace worker context scope with that field before invoking a handler. A legacy
entry without scope receives empty tenant context so business repositories fail
closed. Client/model payload fields never choose the tenant.
Only successful handlers are acknowledged; failures remain pending until reclaim.
XAUTOCLAIM and XPENDING determine delivery count; final failure goes to DLQ.
EnqueueOutbox stores the context request id (or a new `job_` id) in `outbox.request_id`; the relay copies it to the `request_id` stream field; the consumer restores it (a missing or malformed value becomes a new `job_` id). Job outcome logs (`job handled`, `job failed, will retry`, `job moved to DLQ`) carry `duration_ms` and never the payload.
The shared [log field contract](../logging/README.md#log-field-contract) applies to worker and handler logs.
Logs contain catalog error codes and job metadata, never payloads or raw errors.
Handler errors should be safe to persist because the DLQ retains the last error.
The default is four total deliveries: the initial attempt plus up to three retries; see [consumer config](../config/README.md).
The foundation consumer retries handler failures up to its configured limit;
domain handlers must apply retryable-error/backoff policy for outbound operations.

DLQ stream: `<source>.dlq`. Fields: `payload`, `request_id`, `error`,
`deliveries`, `original_id`, `outbox_id`, `tenant_id`. The original outbox ID is preserved
through DLQ delivery so replay retains the same deduplication key.
To replay, inspect the DLQ entry, then XADD to the source stream with its payload,
request_id, preserved outbox_id, and tenant_id. For example:
```bash
redis-cli XADD thomas.ingest.requested '*' payload '<json>' request_id '<req_or_job_id>' outbox_id '<original-id>' tenant_id '<trusted-tenant-id>'
```
Keep the deduplication policy explicit when replaying an already completed job.
The replay UI comes in M4. Replay starts a fresh pending delivery count.

## Common failures
- BUSYGROUP is harmless on EnsureGroup; other group errors are returned.
- Redis or DB failures leave outbox rows unpublished for the next relay cycle.
- DLQ publication failures leave the source message pending for recovery.
- DLQ publish and XACK are separate operations: a crash can duplicate a DLQ entry.

`EnqueueOutbox` returns the inserted id. `PublishOne` expedites that row after its transaction commits; the relay still picks it up if publication fails (a row locked by a batch is skipped by `PublishOne` and published once by that batch).

## Trace context
`EnqueueOutbox` writes the W3C `traceparent` of the context span to `outbox.traceparent` (NULL when there is no span). It uses `propagation.TraceContext{}` directly, not the global propagator, because nothing in core installs a global one. The relay copies the value to the `traceparent` stream field, the consumer exposes it as `Message.TraceParent`, and the DLQ keeps it.

## Consumer concurrency and shutdown
- `Consumer.Concurrency` pollers share one consumer name. Each poller keeps the entry ids it is running in a shared in-flight set; reclaim (`XAUTOCLAIM ... JUSTID`, then `XCLAIM`) skips those ids, so a handler that runs longer than `VisibilityTimeout` is **not** executed a second time by a sibling poller, and the skip does not count as a delivery. Entries behind a slow one in a `BatchSize>1` read are marked in flight too.
- A crashed or other consumer's idle entries are still reclaimed after `VisibilityTimeout`; they count as a new delivery. Handlers must still be idempotent (at-least-once).
- Set `VisibilityTimeout` above the longest expected handler run anyway: another process may reclaim the entry, and the consumer cannot see that.
- On cancel, `Run` stops reading and waits for in-flight handlers up to `DrainTimeout` (`0` waits without limit). Handlers run with a context that is not cancelled by shutdown. When the drain expires `Run` logs a `WARN` with the `in_flight` count and returns while those handlers keep running detached; the caller must not close its pool as if they had finished.
- `OutboxRelay.Run` finishes the batch it has started after cancellation, so XADD and the `published_at` update stay in step.
