import uuid
from collections.abc import Mapping
from typing import Final
from uuid import UUID

from psycopg import AsyncConnection
from psycopg.rows import DictRow
from psycopg.types.json import Jsonb

STREAM_INGEST_REQUESTED: Final = "thomas.ingest.requested"
STREAM_KB_PURGE_REQUESTED: Final = "thomas.kb.purge.requested"
KNOWN_STREAMS: Final = (STREAM_INGEST_REQUESTED, STREAM_KB_PURGE_REQUESTED)
PURGE_KIND_KB: Final = "kb"
PURGE_KIND_FILE: Final = "file"


async def enqueue_outbox(
    conn: AsyncConnection[DictRow], org_id: str, stream: str, payload: Mapping[str, object], request_id: str
) -> UUID:
    """Call inside the business transaction so the job exists exactly when the change does."""
    outbox_id = uuid.uuid4()
    await conn.execute(
        b"INSERT INTO outbox (id, org_id, stream, payload, request_id) VALUES (%s, %s, %s, %s, %s)",
        (outbox_id, org_id, stream, Jsonb(dict(payload)), request_id),
    )
    return outbox_id
