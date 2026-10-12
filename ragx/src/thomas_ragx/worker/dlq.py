from collections.abc import Sequence
from typing import Final, cast

import structlog
from redis.asyncio import Redis

from thomas_ragx.worker.consumer import DEAD_LETTER_SUFFIX
from thomas_ragx.worker.relay import FIELD_ORG_ID, FIELD_OUTBOX_ID, FIELD_PAYLOAD, FIELD_REQUEST_ID

REPLAYED_FIELDS: Final = (FIELD_PAYLOAD, FIELD_REQUEST_ID, FIELD_ORG_ID, FIELD_OUTBOX_ID)
log = structlog.get_logger(__name__)


class DeadLetterReplayer:
    """Moves dead letters back to their stream as fresh entries; the handler's claim makes a rerun safe."""

    def __init__(self, redis: Redis) -> None:
        self._redis = redis

    async def replay(self, stream: str, entry_ids: Sequence[str] | None) -> int:
        dead_letters = stream + DEAD_LETTER_SUFFIX
        entries = cast(list[tuple[str, dict[str, str]]], await self._redis.xrange(dead_letters))
        wanted = None if entry_ids is None else set(entry_ids)
        replayed = 0
        for entry_id, fields in entries:
            if wanted is not None and entry_id not in wanted:
                continue
            await self._redis.xadd(stream, {name: fields.get(name, "") for name in REPLAYED_FIELDS})
            await self._redis.xdel(dead_letters, entry_id)
            replayed += 1
            log.info("dead letter replayed", id=entry_id, stream=stream)
        return replayed
