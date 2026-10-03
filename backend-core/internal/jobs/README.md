# Jobs

## Purpose
Owns transactional outbox enqueueing, Redis stream publishing, pending-message
recovery, acknowledgement, and dead-letter delivery.

## Entry points
- `Enqueue`: call inside the transaction that writes the business change.
- `Relay.Run` / `PublishBatch`: `core relay` publishes committed outbox rows.
- `Consumer.EnsureGroup`, `Run`, `Poll`: worker entry points.
- `Handler`: application processing with the restored W3C trace context.

## Dependencies
- Logger construction returns an error for unknown levels; callers must handle it before injecting a logger.
- Uses: pgx/Postgres, go-redis/Redis Streams, `internal/errors`, OpenTelemetry, slog.
- Used by: core relay and future domain workers.
- Configure batch/poll limits from `config.RelayConfig`; consumer limits from
  `config.StreamConfig`. Callers provide clients and a structured logger.

## Run & test
```bash
cd backend-core
go test ./internal/jobs/... -count=1
go run ./cmd/core relay
```
Docker Desktop must run for the pinned Postgres/Redis integration containers.

## Conventions
Delivery is at-least-once. A crash after XADD and before transaction COMMIT can
publish the same outbox row again. Handlers must deduplicate by `outbox_id` and
make their business side effects idempotent. Handlers receive the original `outbox_id` directly in `Message.OutboxID`
before applying side effects. Entries created outside the outbox may omit this
field; domain handlers must define an appropriate idempotency key for those jobs.
Rows use FOR UPDATE SKIP LOCKED so multiple relays can share the outbox.
Only successful handlers are acknowledged; failures remain pending until reclaim.
XAUTOCLAIM and XPENDING determine delivery count; final failure goes to DLQ.
Enqueue/relay/poll/job boundaries have spans; handler logs use restored trace IDs.
Logs contain catalog error codes and job metadata, never payloads or raw errors.
Handler errors should be safe to persist because the DLQ retains the last error.
The foundation consumer retries handler failures up to its configured limit;
domain handlers must apply retryable-error/backoff policy for outbound operations.

DLQ stream: `<source>.dlq`. Fields: `payload`, `traceparent`, `error`,
`deliveries`, `original_id`, `outbox_id`. The original outbox ID is preserved
through DLQ delivery so replay retains the same deduplication key.
To replay, inspect the DLQ entry, then XADD to the source stream with its payload,
traceparent, and preserved outbox_id. For example:
```bash
redis-cli XADD thomas.ingest.requested '*' payload '<json>' traceparent '<w3c>' outbox_id '<original-id>'
```
Keep the deduplication policy explicit when replaying an already completed job.
The replay UI comes in M4. Replay starts a fresh pending delivery count.

## Common failures
- BUSYGROUP is harmless on EnsureGroup; other group errors are returned.
- Redis or DB failures leave outbox rows unpublished for the next relay cycle.
- DLQ publication failures leave the source message pending for recovery.
- DLQ publish and XACK are separate operations: a crash can duplicate a DLQ entry.
