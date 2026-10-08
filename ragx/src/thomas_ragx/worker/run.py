import asyncio
import os
import socket
from pathlib import Path

import structlog
from opentelemetry.trace import Tracer

from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.tracing import ATTR_ORG_ID, ATTR_REQUEST_ID
from thomas_ragx.resources import Resources
from thomas_ragx.worker.consumer import Handler, StreamConsumer, StreamMessage
from thomas_ragx.worker.handlers import IngestHandler, PurgeHandler
from thomas_ragx.worker.metrics import WorkerMetrics, configure_worker_metrics
from thomas_ragx.worker.relay import OutboxRelay

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


async def _consume(
    consumer: StreamConsumer, handler: Handler, stop: asyncio.Event, heartbeat: Path, pause: float
) -> None:
    while not stop.is_set():
        heartbeat.touch()
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
    consumers = [
        (
            StreamConsumer(
                resources.redis,
                stream,
                name,
                max_deliveries=config.stream_max_deliveries,
                visibility_timeout_ms=config.stream_visibility_timeout_milliseconds,
                block_ms=config.stream_block_milliseconds,
                batch_size=config.stream_batch_size,
                metrics=metrics,
            ),
            _traced(tracer, stream, handler),
        )
        for stream, handler in handlers.items()
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
    heartbeat = Path(config.worker_heartbeat_file)
    log.info("worker started", consumer=name)
    await asyncio.gather(
        relay.run(stop),
        *(_consume(c, h, stop, heartbeat, config.outbox_poll_interval_seconds) for c, h in consumers),
    )
