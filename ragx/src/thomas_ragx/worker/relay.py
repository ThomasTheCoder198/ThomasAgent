import asyncio
import contextlib
import json
from time import perf_counter
from typing import Final, Protocol

import structlog
from redis.asyncio import Redis

from thomas_ragx.db.pool import Pool
from thomas_ragx.worker.metrics import OUTCOME_ERROR, OUTCOME_SUCCESS

FIELD_PAYLOAD: Final = "payload"
FIELD_REQUEST_ID: Final = "request_id"
FIELD_ORG_ID: Final = "org_id"
FIELD_OUTBOX_ID: Final = "outbox_id"
SELECT_UNPUBLISHED: Final = (
    b"SELECT id, org_id, stream, payload, request_id FROM outbox WHERE published_at IS NULL "
    b"ORDER BY created_at LIMIT %s FOR UPDATE SKIP LOCKED"
)
log = structlog.get_logger(__name__)


class RelayRecorder(Protocol):
    def observe_relay(self, outcome: str, duration_seconds: float, rows: int) -> None: ...


class OutboxRelay:
    """At-least-once: a crash between XADD and COMMIT republishes a row, so handlers are idempotent."""

    def __init__(
        self,
        pool: Pool,
        redis: Redis,
        batch_size: int,
        poll_interval_seconds: float,
        metrics: RelayRecorder | None = None,
    ) -> None:
        self._pool = pool
        self._redis = redis
        self._batch_size = batch_size
        self._poll_interval = poll_interval_seconds
        self._metrics = metrics

    def _observe(self, outcome: str, started: float, rows: int) -> None:
        if self._metrics is not None:
            self._metrics.observe_relay(outcome, perf_counter() - started, rows)

    async def publish_batch(self) -> int:
        started = perf_counter()
        try:
            async with self._pool.connection() as conn, conn.transaction():
                rows = await (await conn.execute(SELECT_UNPUBLISHED, (self._batch_size,))).fetchall()
                for row in rows:
                    await self._redis.xadd(
                        row["stream"],
                        {
                            FIELD_PAYLOAD: json.dumps(row["payload"], sort_keys=True),
                            FIELD_REQUEST_ID: row["request_id"],
                            FIELD_ORG_ID: row["org_id"],
                            FIELD_OUTBOX_ID: str(row["id"]),
                        },
                    )
                    await conn.execute(b"UPDATE outbox SET published_at = now() WHERE id = %s", (row["id"],))
        except Exception:
            self._observe(OUTCOME_ERROR, started, 0)
            raise
        self._observe(OUTCOME_SUCCESS, started, len(rows))
        return len(rows)

    async def run(self, stop: asyncio.Event) -> None:
        while not stop.is_set():
            published = 0
            try:
                published = await self.publish_batch()
            except Exception as exc:  # Redis or Postgres outage: retry on the next tick
                log.error("outbox relay batch failed", error_type=type(exc).__name__)  # noqa: TRY400
            if published == 0:
                with contextlib.suppress(TimeoutError):
                    await asyncio.wait_for(stop.wait(), self._poll_interval)
