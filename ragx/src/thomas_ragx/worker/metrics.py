import sys
from typing import Final

from opentelemetry.metrics import Meter
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import (
    ConsoleMetricExporter,
    MetricsData,
    PeriodicExportingMetricReader,
)
from opentelemetry.sdk.metrics.view import ExplicitBucketHistogramAggregation, View
from opentelemetry.sdk.resources import Resource

from thomas_ragx.platform.config import Config

RERANK_CALLS_NAME: Final = "ragx_rerank_calls_total"
RERANK_DURATION_NAME: Final = "ragx_rerank_call_duration_seconds"
CHAT_CALLS_NAME: Final = "ragx_chat_calls_total"
CHAT_DURATION_NAME: Final = "ragx_chat_call_duration_seconds"
EMBEDDING_CALLS_NAME: Final = "ragx_embedding_calls_total"
EMBEDDING_DURATION_NAME: Final = "ragx_embedding_call_duration_seconds"
EMBEDDING_CACHE_NAME: Final = "ragx_embedding_cache_total"
INGEST_STEP_DURATION_NAME: Final = "ragx_ingest_step_duration_seconds"
INGEST_FILES_NAME: Final = "ragx_ingest_files_total"
INGEST_DURATION_BUCKETS_SECONDS: Final = (120.0, 300.0, 600.0)
JOB_COUNT_NAME: Final = "ragx_worker_jobs_total"
JOB_DURATION_NAME: Final = "ragx_worker_job_duration_seconds"
RELAY_BATCH_COUNT_NAME: Final = "ragx_worker_relay_batches_total"
RELAY_DURATION_NAME: Final = "ragx_worker_relay_duration_seconds"
RELAY_ROWS_NAME: Final = "ragx_worker_relay_rows_total"
SWEEP_COUNT_NAME: Final = "ragx_worker_sweep_total"
HTTP_REQUESTS_NAME: Final = "ragx_http_requests_total"
HTTP_DURATION_NAME: Final = "ragx_http_request_duration_seconds"
SWEEP_KIND_SLOT: Final = "slot"
SWEEP_KIND_OBJECT: Final = "object"
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
OUTCOME_RETRY: Final = "retry"
OUTCOME_DEAD_LETTER: Final = "dead_letter"
METRICS_SCOPE: Final = "thomas_ragx.worker"
JOB_DURATION_BUCKETS_SECONDS: Final = (0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0)


class WorkerMetrics:
    """Record bounded job outcomes and duration without request or tenant labels."""

    def __init__(self, meter: Meter) -> None:
        self._rerank_calls = meter.create_counter(RERANK_CALLS_NAME)
        self._rerank_duration = meter.create_histogram(RERANK_DURATION_NAME, unit="s")
        self._chat_calls = meter.create_counter(CHAT_CALLS_NAME)
        self._chat_duration = meter.create_histogram(CHAT_DURATION_NAME, unit="s")
        self._embedding_calls = meter.create_counter(EMBEDDING_CALLS_NAME)
        self._embedding_duration = meter.create_histogram(EMBEDDING_DURATION_NAME, unit="s")
        self._embedding_cache = meter.create_counter(EMBEDDING_CACHE_NAME)
        self._ingest_steps = meter.create_histogram(INGEST_STEP_DURATION_NAME, unit="s")
        self._ingest_files = meter.create_counter(INGEST_FILES_NAME)
        self._jobs = meter.create_counter(JOB_COUNT_NAME, description="Worker job delivery attempts")
        self._duration = meter.create_histogram(
            JOB_DURATION_NAME, unit="s", description="Worker job delivery duration"
        )
        self._relay_batches = meter.create_counter(RELAY_BATCH_COUNT_NAME, description="Outbox relay batches")
        self._relay_duration = meter.create_histogram(
            RELAY_DURATION_NAME, unit="s", description="Outbox relay batch duration"
        )
        self._relay_rows = meter.create_counter(RELAY_ROWS_NAME, description="Outbox rows published")
        self._sweeps = meter.create_counter(SWEEP_COUNT_NAME, description="Items the sweeper deleted")

    def observe_rerank_call(self, kind: str, outcome: str, duration_seconds: float) -> None:
        attributes = {"kind": kind, "outcome": outcome}
        self._rerank_calls.add(1, attributes)
        self._rerank_duration.record(duration_seconds, attributes)

    def observe_chat_call(self, kind: str, outcome: str, duration_seconds: float) -> None:
        attributes = {"kind": kind, "outcome": outcome}
        self._chat_calls.add(1, attributes)
        self._chat_duration.record(duration_seconds, attributes)

    def observe_embedding_call(self, kind: str, outcome: str, duration_seconds: float) -> None:
        attributes = {"kind": kind, "outcome": outcome}
        self._embedding_calls.add(1, attributes)
        self._embedding_duration.record(duration_seconds, attributes)

    def observe_embedding_cache(self, result: str, count: int) -> None:
        if count > 0:
            self._embedding_cache.add(count, {"result": result})

    def observe_ingest_step(self, step: str, duration_seconds: float) -> None:
        self._ingest_steps.record(duration_seconds, {"step": step})

    def observe_ingest_file(self, outcome: str) -> None:
        self._ingest_files.add(1, {"outcome": outcome})

    def observe_job(self, stream: str, outcome: str, duration_seconds: float) -> None:
        attributes = {"stream": stream, "outcome": outcome}
        self._jobs.add(1, attributes)
        self._duration.record(duration_seconds, attributes)

    def observe_relay(self, outcome: str, duration_seconds: float, rows: int) -> None:
        attributes = {"outcome": outcome}
        self._relay_batches.add(1, attributes)
        self._relay_duration.record(duration_seconds, attributes)
        if rows:
            self._relay_rows.add(rows)

    def observe_sweep(self, kind: str, count: int) -> None:
        if count > 0:
            self._sweeps.add(count, {"kind": kind})


def _one_line(data: MetricsData) -> str:
    # The SDK default is indented JSON; stdout carries one log record per line, so keep exports on one line.
    return data.to_json(indent=None) + "\n"


def configure_worker_metrics(config: Config) -> tuple[MeterProvider, WorkerMetrics]:
    # Shared by the worker and the API process: neither exposes a scrape endpoint, so periodic stdout export
    # makes metrics visible in container logs.
    reader = PeriodicExportingMetricReader(
        ConsoleMetricExporter(out=sys.stdout, formatter=_one_line),
        export_interval_millis=config.worker_metrics_export_interval_milliseconds,
    )
    provider = MeterProvider(
        metric_readers=[reader],
        resource=Resource.create(
            {"service.name": config.service_name, "deployment.environment": config.environment}
        ),
        views=[
            View(
                instrument_name=INGEST_STEP_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(
                    JOB_DURATION_BUCKETS_SECONDS + INGEST_DURATION_BUCKETS_SECONDS
                ),
            ),
            View(
                instrument_name=JOB_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(JOB_DURATION_BUCKETS_SECONDS),
            ),
            View(
                instrument_name=RELAY_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(JOB_DURATION_BUCKETS_SECONDS),
            ),
            # The SDK default boundaries are millisecond-scale; this histogram is recorded in seconds.
            View(
                instrument_name=HTTP_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(JOB_DURATION_BUCKETS_SECONDS),
            ),
        ],
    )
    return provider, WorkerMetrics(provider.get_meter(METRICS_SCOPE))
