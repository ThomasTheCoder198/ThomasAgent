import sys
from typing import Final

from opentelemetry.metrics import Meter
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import ConsoleMetricExporter, PeriodicExportingMetricReader
from opentelemetry.sdk.metrics.view import ExplicitBucketHistogramAggregation, View
from opentelemetry.sdk.resources import Resource

from thomas_ragx.platform.config import Config

JOB_COUNT_NAME: Final = "ragx_worker_jobs_total"
JOB_DURATION_NAME: Final = "ragx_worker_job_duration_seconds"
RELAY_BATCH_COUNT_NAME: Final = "ragx_worker_relay_batches_total"
RELAY_DURATION_NAME: Final = "ragx_worker_relay_duration_seconds"
RELAY_ROWS_NAME: Final = "ragx_worker_relay_rows_total"
OUTCOME_SUCCESS: Final = "success"
OUTCOME_ERROR: Final = "error"
OUTCOME_RETRY: Final = "retry"
OUTCOME_DEAD_LETTER: Final = "dead_letter"
METRICS_SCOPE: Final = "thomas_ragx.worker"
JOB_DURATION_BUCKETS_SECONDS: Final = (0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0)


class WorkerMetrics:
    """Record bounded job outcomes and duration without request or tenant labels."""

    def __init__(self, meter: Meter) -> None:
        self._jobs = meter.create_counter(JOB_COUNT_NAME, description="Worker job delivery attempts")
        self._duration = meter.create_histogram(
            JOB_DURATION_NAME, unit="s", description="Worker job delivery duration"
        )
        self._relay_batches = meter.create_counter(RELAY_BATCH_COUNT_NAME, description="Outbox relay batches")
        self._relay_duration = meter.create_histogram(
            RELAY_DURATION_NAME, unit="s", description="Outbox relay batch duration"
        )
        self._relay_rows = meter.create_counter(RELAY_ROWS_NAME, description="Outbox rows published")

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


def configure_worker_metrics(config: Config) -> tuple[MeterProvider, WorkerMetrics]:
    # The worker has no HTTP server; periodic stdout export makes metrics visible in container logs.
    reader = PeriodicExportingMetricReader(
        ConsoleMetricExporter(out=sys.stdout),
        export_interval_millis=config.worker_metrics_export_interval_milliseconds,
    )
    provider = MeterProvider(
        metric_readers=[reader],
        resource=Resource.create(
            {"service.name": config.service_name, "deployment.environment": config.environment}
        ),
        views=[
            View(
                instrument_name=JOB_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(JOB_DURATION_BUCKETS_SECONDS),
            ),
            View(
                instrument_name=RELAY_DURATION_NAME,
                aggregation=ExplicitBucketHistogramAggregation(JOB_DURATION_BUCKETS_SECONDS),
            ),
        ],
    )
    return provider, WorkerMetrics(provider.get_meter(METRICS_SCOPE))
