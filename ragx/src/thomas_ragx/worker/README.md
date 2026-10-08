# Worker

## Purpose
Publishes ragx's outbox to ragx Redis and consumes `thomas.ingest.requested` and `thomas.kb.purge.requested` with retries and a DLQ.

## Entry points
- `ragx worker` starts the process through `run_worker`.
- `OutboxRelay` publishes committed outbox rows.
- `StreamConsumer` reads, retries, acknowledges, and dead letters jobs.
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
Delivery is at least once: a crash between XADD and COMMIT republishes, so handlers are idempotent. Stream fields are `payload`, `request_id`, `org_id`, and `outbox_id`; the DLQ adds `error` (catalog code or exception type, never a message), `deliveries`, and `original_id`. A malformed request ID becomes a `job_` ID. The consumer group is `ragx-worker`; after `RAG_STREAM_MAX_DELIVERIES` deliveries, entries move to `<stream>.dlq`. Jobs run in `rag.ingest.receive` or `rag.kb.purge` spans with request ID and org ID. `RAG_WORKER_HEARTBEAT_FILE` is touched every poll for the compose healthcheck. The M1.1 ingest handler checks the raw object only; M1.3 replaces it.

OpenTelemetry metrics `ragx_worker_jobs_total` and `ragx_worker_job_duration_seconds` use only `stream` and `outcome` labels (`success`, `retry`, `dead_letter`). Relay metrics `ragx_worker_relay_batches_total` and `ragx_worker_relay_duration_seconds` use bounded `outcome` labels (`success`, `error`); `ragx_worker_relay_rows_total` counts published rows. The worker exports one-line JSON to stdout every `RAG_WORKER_METRICS_EXPORT_INTERVAL_MILLISECONDS`, visible through container logs. Request IDs never become metric labels.

## Common failures
- `outbox relay batch failed` means ragx Redis is down. Rows remain unpublished until Redis returns.
- Entries in `*.dlq` need inspection by error code, then replay with XADD to the original stream.
