# Metrics

## Purpose
Owns core's Prometheus registry and every metric core exports: HTTP RED (with `request_id` exemplars), outbound
calls, outbox backlog, stream jobs, LLM calls, and the Go/process runtime collectors. It does not decide where
metrics are served (`cmd/core`) or when they are recorded (the instrumented packages).

## Entry points
- `New(httpBuckets, llmBuckets)` — `cmd/core` at startup, from `config.MetricsConfig`.
- `Handler()` — the internal `/metrics` listener (OpenMetrics, so exemplars survive).
- `RegisterJobStream(stream)` — consumer startup emits the DLQ counter at zero before failures occur.
- `ObserveChatRunQueue` — `chat_run_queue_seconds`, the wait between a chat run's outbox insert and a worker's claim (no labels), called by `chat/worker`.
- `ObserveHTTP`, `ObserveOutbound`, `ObserveJob`, `SetOutboxBacklog`, `ObserveLLMCall` — called through small
  interfaces declared by `httpx`, `llmprovider`, `outbound`, `jobs` and `calllog`.

## Dependencies
- Uses: `github.com/prometheus/client_golang` v1.24.1. Must not import `config` (config imports outbound, which uses
  this package's outcome constants).
- Used by: `cmd/core`, `httpx`, `llmprovider`, `outbound`, `jobs`, `calllog`.

## Run & test
```bash
cd backend-core
go test ./internal/platform/metrics -count=1
```

## Conventions
Labels take bounded values only (spec 2026-10-06 §5): method (unknown verbs become `OTHER`), route pattern or
`unmatched`, status class, provider kind, model, role, outcome, token kind, target, stream. Never `request_id`,
`user_id` or `tenant_id`; the request id is a histogram exemplar. `llm_cost_usd_total` is for dashboards;
`llm_usage` in Postgres is the source of truth for cost.

Consumers register their configured stream before processing jobs. This creates a zero-valued
`jobs_dlq_total` series so Prometheus can observe the first dead-letter increment; registration is
idempotent and preserves an existing counter. Prometheus must scrape the baseline before the increment
to count that event with `increase`.

## Common failures
- Missing request metrics → inject the registry into the HTTP, outbound and job boundaries; registration alone does not record calls.
- No exemplars in Grafana → scrape not in OpenMetrics or Prometheus started without `--enable-feature=exemplar-storage`.
