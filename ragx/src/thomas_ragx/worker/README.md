# Worker

## Purpose
Publishes ragx's outbox to ragx Redis and consumes `thomas.ingest.requested` and `thomas.kb.purge.requested` with retries and a DLQ, and sweeps expired upload slots.

## Entry points
- `ragx worker` starts the process through `run_worker`.
- `OutboxRelay` publishes committed outbox rows.
- `StreamConsumer` reads, retries, acknowledges, and dead letters jobs; `on_dead_letter` is its failure hook and `retry_delay` its backoff.
- `plan.consumer_plan` lists the consumers: `RAG_INGEST_CONCURRENCY` ingest consumers plus one purge consumer.
- `run.heartbeat` touches the heartbeat file on its own timer.
- `IngestDeadLetter` is the ingest dead-letter hook: it marks the file `failed` with a catalog code.
- `dlq.DeadLetterReplayer` backs `ragx dlq replay`.
- `sweeper.Sweeper` removes expired upload slots and row-less objects.
- `IngestHandler` receives uploads; `PurgeHandler` removes deleted files and KBs.
- `WorkerMetrics` records job attempts and duration; `configure_worker_metrics` exports them to stdout.

## Dependencies
- Uses: ragx Postgres, ragx Redis, MinIO.
- Used by: ragx worker process; mirrors `backend-core/internal/platform/jobs`.

## Run & test
```bash
cd ragx && uv run pytest -q tests/integration/test_worker.py
```

## Conventions
Delivery is at least once: a crash between XADD and COMMIT republishes, so handlers are idempotent. Stream fields are `payload`, `request_id`, `org_id`, and `outbox_id`; the DLQ adds `error` (catalog code or exception type, never a message), `deliveries`, and `original_id`. A malformed request ID becomes a `job_` ID. The consumer group is `ragx-worker`; after `RAG_STREAM_MAX_DELIVERIES` deliveries, entries move to `<stream>.dlq`. Jobs run in `rag.ingest.receive` or `rag.kb.purge` spans with request ID and org ID. `RAG_WORKER_HEARTBEAT_FILE` is touched by the heartbeat task for the compose healthcheck. The M1.1 ingest handler checks the raw object only; M1.3 replaces it.

Concurrency is `RAG_INGEST_CONCURRENCY` sequential consumers (one consumer name each) on the ingest stream; the purge stream has one. Ingest consumers read one entry at a time (`plan.INGEST_BATCH_SIZE`, `ConsumerSpec.batch_size`), so two slots really run two jobs and the visibility timeout is counted per job; a batch would have one consumer work up to `RAG_STREAM_BATCH_SIZE` jobs alone while the other idles, and the batch's last entry could outwait the visibility timeout and be reclaimed while healthy. The purge consumer keeps `RAG_STREAM_BATCH_SIZE`. The ingest consumers use `RAG_INGEST_VISIBILITY_TIMEOUT_MILLISECONDS` (30 min), longer than two bounded Datalab converts (config validation, ruling A11), so a healthy slow job is never reclaimed; the purge consumer keeps `RAG_STREAM_VISIBILITY_TIMEOUT_MILLISECONDS`. Visibility now matters only for crash recovery.

Retry rules (rule 5, rulings A12 and A25):
- A non-retryable `AppError` (catalog `retryable: false`) goes to the DLQ on its first delivery. A retryable one, or any other exception (driver, network), retries up to `RAG_STREAM_MAX_DELIVERIES`.
- A retry does not wait for the visibility timeout: the consumer sleeps a full-jitter backoff (`RAG_RETRY_BASE_DELAY_S`, `RAG_RETRY_MAX_DELAY_S`; `platform.retry.backoff_seconds`), re-adds the entry with an `attempt` field plus one, then acknowledges the original. The delivery number is `max(attempt, times_delivered)`, so a crash-reclaimed entry still counts. The consumer slot sleeps for the backoff (seconds).
- `on_dead_letter(message, error_code)` runs after the DLQ write and before the ack; a failing hook is logged and the entry is acknowledged anyway. The ingest hook sets the file `failed` and its `error_code` unless it is `ready`, `degraded` or an unconfirmed slot (then it logs `ingest dead letter ignored, file finished`); an exception type name in the DLQ becomes `INTERNAL_ERROR` on the row.

The heartbeat is its own task (`RAG_WORKER_HEARTBEAT_INTERVAL_SECONDS`, 10 s, under the compose check's 60 s), so a long ingest job does not make the container look dead.

The sweeper runs every `RAG_SWEEP_INTERVAL_SECONDS` (600) under a `job_` request id and logs `sweep finished` with `duration_ms`. With the cutoff `RAG_UPLOAD_URL_TTL + RAG_UPLOAD_SWEEP_GRACE_SECONDS` it deletes `uploading` rows older than the cutoff (age judged by the database clock) and, only for a row it actually deleted, the object (the row goes first: the `DELETE` waits on a concurrent confirm's row lock and then matches nothing, so a slot that confirm won keeps its object and is not counted; a failed object delete is picked up by the next object pass), then deletes `kb-raw` and `kb-preview` objects older than the cutoff whose file id has no `kb_files` row (ruling A16). Confirmed files and fresh slots are never touched. Metric `ragx_worker_sweep_total{kind=slot|object}` counts deleted items.

OpenTelemetry metrics `ragx_worker_jobs_total` and `ragx_worker_job_duration_seconds` use only `stream` and `outcome` labels (`success`, `retry`, `dead_letter`). Relay metrics `ragx_worker_relay_batches_total` and `ragx_worker_relay_duration_seconds` use bounded `outcome` labels (`success`, `error`); `ragx_worker_relay_rows_total` counts published rows. The worker exports one-line JSON to stdout every `RAG_WORKER_METRICS_EXPORT_INTERVAL_MILLISECONDS`, visible through container logs. Request IDs never become metric labels.

## Common failures
- `outbox relay batch failed` means ragx Redis is down. Rows remain unpublished until Redis returns.
- Entries in `*.dlq` need inspection by error code, then replay: `ragx dlq replay <stream> --all` or `--entry-id <id> [--entry-id <id> ...]` (run it in the worker container; streams are `thomas.ingest.requested` and `thomas.kb.purge.requested`). It moves entries back with their original `payload`, `request_id`, `org_id` and `outbox_id`, and the delivery count starts again at 1; the claim in the handler keeps a rerun safe (ruling A14).
- `sweep failed` means MinIO or Postgres was unreachable; the next period retries.

IngestHandler now invokes the ingest pipeline; its enclosing span is `rag.ingest.run`. Step duration and file outcome metrics are `ragx_ingest_step_duration_seconds` and `ragx_ingest_files_total`.

## Embedding cache metrics
`WorkerMetrics.observe_embedding_cache(result, count)` increments `ragx_embedding_cache_total` for positive counts with bounded `result=hit|miss` labels.

`WorkerMetrics.observe_embedding_call(kind, outcome, duration_seconds)` records embedding outbound counts and durations through the worker's configured meter/exporter. Pass this recorder explicitly to `LiteLLMEmbedder(..., metrics=metrics)` when constructing the indexing pipeline.

`WorkerMetrics.observe_chat_call(kind, outcome, duration_seconds)` records `ragx_chat_calls_total` and `ragx_chat_call_duration_seconds` through the configured worker exporter. Only bounded kind and success/error outcome labels are used. Task 3b-6 supplies this recorder to `LiteLLMChat` when assembling the summary pipeline.

Purge resolves the KB collection within the signed org scope, deletes file or KB points using tenant filters, then removes raw/preview objects and database rows. Replayed purges tolerate an already removed KB.

Purge now holds the same KB row lock as ingestion publication until point/object deletion and row deletion finish. Lock order is KB before files/nodes; a missing KB pointer is safe because a publisher must acquire and validate this KB before writing points.
