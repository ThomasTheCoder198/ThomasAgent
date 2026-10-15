import asyncio
import contextlib
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from time import perf_counter
from typing import Final, Protocol
from uuid import UUID

import structlog

from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import file_repository as files
from thomas_ragx.kb.storage import PREVIEW_BUCKET, PREVIEW_SUFFIX, RAW_BUCKET, ObjectStorage, raw_key
from thomas_ragx.platform.request_id import new_job_id
from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.worker.metrics import (
    SWEEP_KIND_INGEST_FAILED,
    SWEEP_KIND_INGEST_REQUEUED,
    SWEEP_KIND_OBJECT,
    SWEEP_KIND_SLOT,
)
from thomas_ragx.worker.recovery import IngestRecovery, RecoveryResult

SWEPT_BUCKETS: Final = (RAW_BUCKET, PREVIEW_BUCKET)
KEY_SEPARATOR: Final = "/"
log = structlog.get_logger(__name__)


class SweepRecorder(Protocol):
    def observe_sweep(self, kind: str, count: int) -> None: ...


@dataclass(frozen=True)
class SweepResult:
    slots: int
    objects: int
    requeued: int = 0
    failed: int = 0


def _utc_now() -> datetime:
    return datetime.now(UTC)


def file_id_of(key: str) -> UUID | None:
    name = key.rsplit(KEY_SEPARATOR, 1)[-1].removesuffix(PREVIEW_SUFFIX)
    try:
        return UUID(name)
    except ValueError:
        return None  # not a key ragx wrote: leave it alone


class Sweeper:
    """Removes expired upload slots and objects no row owns (spec §14.3 step 5, ruling A16) and, with an
    `IngestRecovery`, re-drives or fails stalled ingests (m1/m3).
    """

    def __init__(
        self,
        pool: Pool,
        storage: ObjectStorage,
        *,
        cutoff_age: timedelta,
        interval_seconds: float,
        metrics: SweepRecorder | None = None,
        now: Callable[[], datetime] = _utc_now,
        recovery: IngestRecovery | None = None,
    ) -> None:
        self._recovery = recovery  # stalled ingest recovery (m1/m3); None in slot/object-only tests
        self._pool = pool
        self._storage = storage
        self._cutoff_age = cutoff_age
        self._interval = interval_seconds
        self._metrics = metrics
        self._now = now

    async def _sweep_slots(self) -> int:
        async with self._pool.connection() as conn:
            slots = await files.expired_upload_slots(conn, self._cutoff_age.total_seconds())
        swept = 0
        for slot in slots:
            # Row first: the DELETE waits on a concurrent confirm and then matches nothing, so a slot that
            # confirm won keeps its object. If the object delete fails, the next object pass removes it.
            async with self._pool.connection() as conn:
                deleted = await files.delete_upload_slot(conn, slot.id)
            if deleted:
                await self._storage.remove(RAW_BUCKET, raw_key(slot.org_id, slot.kb_id, slot.id))
                swept += 1
        return swept

    async def _sweep_objects(self) -> int:
        cutoff = self._now() - self._cutoff_age
        removed = 0
        for bucket in SWEPT_BUCKETS:
            old = [
                (obj.key, file_id_of(obj.key))
                for obj in await self._storage.list_objects(bucket)
                if obj.last_modified < cutoff
            ]
            candidates = [(key, file_id) for key, file_id in old if file_id is not None]
            if not candidates:
                continue
            async with self._pool.connection() as conn:
                owned = await files.existing_file_ids(conn, [file_id for _, file_id in candidates])
            for key, file_id in candidates:
                if file_id not in owned:
                    await self._storage.remove(bucket, key)
                    removed += 1
        return removed

    async def _recover(self) -> RecoveryResult:
        if self._recovery is None:
            return RecoveryResult(0, 0)
        try:
            return await self._recovery.recover()
        except Exception as exc:  # isolated: a recovery outage must not cost the slot and object sweeps
            log.error("ingest recovery failed", error_type=type(exc).__name__)  # noqa: TRY400
            return RecoveryResult(0, 0)

    async def sweep_once(self) -> SweepResult:
        slots = await self._sweep_slots()
        objects = await self._sweep_objects()
        recovered = await self._recover()
        result = SweepResult(
            slots=slots, objects=objects, requeued=recovered.requeued, failed=recovered.failed
        )
        if self._metrics is not None:
            self._metrics.observe_sweep(SWEEP_KIND_SLOT, result.slots)
            self._metrics.observe_sweep(SWEEP_KIND_OBJECT, result.objects)
            self._metrics.observe_sweep(SWEEP_KIND_INGEST_REQUEUED, result.requeued)
            self._metrics.observe_sweep(SWEEP_KIND_INGEST_FAILED, result.failed)
        return result

    async def run(self, stop: asyncio.Event) -> None:
        while not stop.is_set():
            with structlog.contextvars.bound_contextvars(request_id=new_job_id()):
                started = perf_counter()
                try:
                    result = await self.sweep_once()
                    log.info(
                        "sweep finished",
                        slots=result.slots,
                        objects=result.objects,
                        requeued=result.requeued,
                        failed=result.failed,
                        duration_ms=elapsed_ms(started),
                    )
                except Exception as exc:  # storage or database outage: try again next period
                    log.error(  # noqa: TRY400
                        "sweep failed",
                        error_type=type(exc).__name__,
                        duration_ms=elapsed_ms(started),
                    )
            with contextlib.suppress(TimeoutError):
                await asyncio.wait_for(stop.wait(), self._interval)
