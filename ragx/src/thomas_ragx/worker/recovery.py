import json
from dataclasses import dataclass
from typing import Final

import structlog
from redis.asyncio import Redis

from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, enqueue_outbox
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest import repository as ingest
from thomas_ragx.ingest.job import FIELD_FILE_ID, FIELD_KB_ID, FIELD_VERSION
from thomas_ragx.ingest.repository import StalledFile
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.request_id import new_job_id
from thomas_ragx.worker.consumer import DEAD_LETTER_SUFFIX, FIELD_DELIVERIES, FIELD_ERROR, FIELD_ORIGINAL_ID
from thomas_ragx.worker.relay import FIELD_ORG_ID, FIELD_OUTBOX_ID, FIELD_PAYLOAD, FIELD_REQUEST_ID

# The attempt died without a catalog error (crash, lost release); nothing more specific is known.
STALLED_ERROR: Final = ErrorCode.INTERNAL_ERROR
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class RecoveryResult:
    requeued: int
    failed: int


def _payload(file: StalledFile) -> dict[str, object]:
    # The same shape as the confirm and retry messages, so the normal ingest handler runs it.
    return {FIELD_KB_ID: str(file.kb_id), FIELD_FILE_ID: str(file.file_id), FIELD_VERSION: file.version}


class IngestRecovery:
    """Re-drives in-progress files whose attempt vanished and that no stream entry will retry (m1/m3).

    A file is stalled when its lease ended more than `grace_seconds` ago: a live attempt always ends before
    its lease (job deadline), and the grace leaves the stream's own crash reclaim the first chance. Each
    recovery re-enqueues the normal ingest message through the outbox; after `max_recoveries` the file is
    failed and a dead letter is written, so it can be replayed or retried like any other final failure.
    """

    def __init__(self, pool: Pool, redis: Redis, *, grace_seconds: float, max_recoveries: int) -> None:
        self._pool = pool
        self._redis = redis
        self._grace = grace_seconds
        self._max = max_recoveries

    async def recover(self) -> RecoveryResult:
        async with self._pool.connection() as conn, conn.transaction():
            requeued = await ingest.requeue_stalled(conn, self._grace, self._max)
            request_ids = [new_job_id() for _ in requeued]
            for file, request_id in zip(requeued, request_ids, strict=True):
                await enqueue_outbox(conn, file.org_id, STREAM_INGEST_REQUESTED, _payload(file), request_id)
        for file, request_id in zip(requeued, request_ids, strict=True):  # logged only once committed
            log.warning(
                "stalled ingest requeued",
                request_id=request_id,
                org_id=file.org_id,
                kb_id=str(file.kb_id),
                file_id=str(file.file_id),
                recoveries=file.recoveries,
            )
        async with self._pool.connection() as conn:
            failed = await ingest.fail_stalled(conn, self._grace, self._max, STALLED_ERROR.value)
        for file in failed:
            await self._dead_letter(file)
        return RecoveryResult(requeued=len(requeued), failed=len(failed))

    async def _dead_letter(self, file: StalledFile) -> None:
        request_id = new_job_id()
        try:
            await self._write_dead_letter(file, request_id)
        except (
            Exception
        ) as exc:  # Redis down: the row is failed and retryable anyway; keep the other files going
            log.error(  # noqa: TRY400
                "stalled ingest dead letter failed",
                request_id=request_id,
                file_id=str(file.file_id),
                error_type=type(exc).__name__,
            )
        log.error(
            "stalled ingest failed",
            request_id=request_id,
            org_id=file.org_id,
            kb_id=str(file.kb_id),
            file_id=str(file.file_id),
            recoveries=file.recoveries,
            error_code=STALLED_ERROR.value,
        )

    async def _write_dead_letter(self, file: StalledFile, request_id: str) -> None:
        # The row is already failed (retryable from the UI); the dead letter adds the replay path.
        await self._redis.xadd(
            STREAM_INGEST_REQUESTED + DEAD_LETTER_SUFFIX,
            {
                FIELD_PAYLOAD: json.dumps(_payload(file), sort_keys=True),
                FIELD_REQUEST_ID: request_id,
                FIELD_ORG_ID: file.org_id,
                FIELD_OUTBOX_ID: "",
                FIELD_ERROR: STALLED_ERROR.value,
                FIELD_DELIVERIES: str(file.recoveries),
                FIELD_ORIGINAL_ID: "",
            },
        )
