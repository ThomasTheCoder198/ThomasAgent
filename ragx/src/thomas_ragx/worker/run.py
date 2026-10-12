import asyncio
import contextlib
import os
import socket
from datetime import timedelta
from pathlib import Path

import structlog
from opentelemetry.trace import Tracer

from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.retry import backoff_seconds
from thomas_ragx.platform.tracing import ATTR_ORG_ID, ATTR_REQUEST_ID
from thomas_ragx.resources import Resources
from thomas_ragx.worker.consumer import Handler, StreamConsumer, StreamMessage
from thomas_ragx.worker.handlers import IngestDeadLetter, IngestHandler, PurgeHandler
from thomas_ragx.worker.metrics import WorkerMetrics, configure_worker_metrics
from thomas_ragx.worker.plan import consumer_plan
from thomas_ragx.worker.relay import OutboxRelay
from thomas_ragx.worker.sweeper import Sweeper

SPAN_NAMES = {STREAM_INGEST_REQUESTED: "rag.ingest.receive", STREAM_KB_PURGE_REQUESTED: "rag.kb.purge"}
log = structlog.get_logger(__name__)


def _traced(tracer: Tracer, stream: str, handler: Handler) -> Handler:
    async def run(message: StreamMessage) -> None:
        with tracer.start_as_current_span(
            SPAN_NAMES[stream],
            record_exception=False,
            set_status_on_exception=False,
            attributes={ATTR_REQUEST_ID: message.request_id, ATTR_ORG_ID: message.org_id},
        ):
            await handler(message)

    return run


async def heartbeat(path: Path, interval_seconds: float, stop: asyncio.Event) -> None:
    """Separate from polling: one long ingest job must not make the container look dead."""
    while not stop.is_set():
        path.touch()
        with contextlib.suppress(TimeoutError):
            await asyncio.wait_for(stop.wait(), interval_seconds)


async def _consume(consumer: StreamConsumer, handler: Handler, stop: asyncio.Event, pause: float) -> None:
    while not stop.is_set():
        try:
            await consumer.poll_once(handler)
        except Exception as exc:  # Redis outage: log once per poll and back off
            log.error("stream poll failed", stream=consumer.stream, error_type=type(exc).__name__)  # noqa: TRY400
            await asyncio.sleep(pause)


async def run_worker(config: Config, resources: Resources, tracer: Tracer, stop: asyncio.Event) -> None:
    metric_provider, metrics = configure_worker_metrics(config)
    try:
        await _run_worker(config, resources, tracer, stop, metrics)
    finally:
        metric_provider.shutdown()


async def _run_worker(
    config: Config, resources: Resources, tracer: Tracer, stop: asyncio.Event, metrics: WorkerMetrics
) -> None:
    name = f"{socket.gethostname()}-{os.getpid()}"
    handlers: dict[str, Handler] = {
        STREAM_INGEST_REQUESTED: IngestHandler(resources.pool, resources.storage),
        STREAM_KB_PURGE_REQUESTED: PurgeHandler(resources.pool, resources.storage),
    }
    dead_letter_hooks = {STREAM_INGEST_REQUESTED: IngestDeadLetter(resources.pool)}
    consumers = [
        (
            StreamConsumer(
                resources.redis,
                spec.stream,
                spec.consumer_name,
                max_deliveries=config.stream_max_deliveries,
                visibility_timeout_ms=spec.visibility_timeout_ms,
                block_ms=config.stream_block_milliseconds,
                batch_size=spec.batch_size,
                metrics=metrics,
                on_dead_letter=dead_letter_hooks.get(spec.stream),
                retry_delay=lambda attempt: backoff_seconds(
                    attempt, config.retry_base_delay_seconds, config.retry_max_delay_seconds
                ),
            ),
            _traced(tracer, spec.stream, handlers[spec.stream]),
        )
        for spec in consumer_plan(config, name)
    ]
    for consumer, _ in consumers:
        await consumer.ensure_group()
    relay = OutboxRelay(
        resources.pool,
        resources.redis,
        config.outbox_batch_size,
        config.outbox_poll_interval_seconds,
        metrics=metrics,
    )
    sweeper = Sweeper(
        resources.pool,
        resources.storage,
        cutoff_age=config.upload_url_ttl + timedelta(seconds=config.upload_sweep_grace_seconds),
        interval_seconds=config.sweep_interval_seconds,
        metrics=metrics,
    )
    log.info("worker started", consumer=name, ingest_concurrency=config.ingest_concurrency)
    await asyncio.gather(
        relay.run(stop),
        heartbeat(Path(config.worker_heartbeat_file), config.worker_heartbeat_interval_seconds, stop),
        sweeper.run(stop),
        *(_consume(c, h, stop, config.outbox_poll_interval_seconds) for c, h in consumers),
    )
