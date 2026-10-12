from dataclasses import dataclass
from typing import Final

from thomas_ragx.db.outbox import STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED
from thomas_ragx.platform.config import Config

# One job per read: a consumer that took a batch would work it alone while its sibling idles, and the
# batch's last entry would wait behind long jobs past the visibility timeout.
INGEST_BATCH_SIZE: Final = 1


@dataclass(frozen=True)
class ConsumerSpec:
    stream: str
    consumer_name: str
    visibility_timeout_ms: int
    batch_size: int


def consumer_plan(config: Config, host_name: str) -> list[ConsumerSpec]:
    """Concurrency is N sequential consumers on the ingest stream: Redis hands each one its own entries."""
    ingest = [
        ConsumerSpec(
            STREAM_INGEST_REQUESTED,
            f"{host_name}-ingest-{slot}",
            config.ingest_visibility_timeout_milliseconds,
            INGEST_BATCH_SIZE,
        )
        for slot in range(config.ingest_concurrency)
    ]
    purge = ConsumerSpec(
        STREAM_KB_PURGE_REQUESTED,
        f"{host_name}-purge",
        config.stream_visibility_timeout_milliseconds,
        config.stream_batch_size,
    )
    return [*ingest, purge]
