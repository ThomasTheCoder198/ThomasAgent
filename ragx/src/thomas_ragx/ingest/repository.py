from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import StrEnum
from typing import Final
from uuid import UUID

from psycopg.rows import DictRow
from psycopg.types.json import Jsonb

from thomas_ragx.db.outbox import PURGE_KIND_FILE, STREAM_KB_PURGE_REQUESTED, enqueue_outbox
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.ingest.outline import Outline
from thomas_ragx.kb.model import FileStatus
from thomas_ragx.kb.repository import Connection
from thomas_ragx.parsing.model import Block, ParsedDocument

IN_PROGRESS: Final = (
    FileStatus.PARSING,
    FileStatus.ENRICHING,
    FileStatus.CHUNKING,
    FileStatus.EMBEDDING,
    FileStatus.INDEXING,
)
CLAIMABLE: Final = (FileStatus.PENDING, FileStatus.FAILED)
CLAIM: Final = (
    b"UPDATE kb_files f SET status = 'parsing', error_code = '', updated_at = now(), "
    b"ingest_lease_until = now() + make_interval(secs => %s), ingest_attempt_id = gen_random_uuid(), "
    # A claim from failed (UI retry, DLQ replay) starts a fresh recovery budget (worker.recovery).
    b"ingest_recoveries = CASE WHEN f.status = 'failed' THEN 0 ELSE f.ingest_recoveries END "
    b"FROM knowledge_bases kb, orgs o "
    b"WHERE f.id = %s AND f.org_id = %s AND f.kb_id = %s AND f.version = %s AND f.deleted_at IS NULL "
    b"AND kb.id = f.kb_id AND kb.deleted_at IS NULL AND o.id = f.org_id "
    b"AND (f.status = ANY(%s) OR (f.status = ANY(%s) AND f.ingest_lease_until < now())) "
    b"RETURNING f.id, f.org_id, f.kb_id, f.version, f.name, f.sha256, o.external_parsing, "
    b"kb.collection_name, kb.embedding_model_id, kb.embedding_dims, f.ingest_attempt_id"
)
# Matches only the attempt that still holds the claim (Codex I1); a stale attempt's write matches no row.
OWNED_BY_ATTEMPT: Final = (
    b"id = %s AND version = %s AND ingest_attempt_id = %s AND deleted_at IS NULL AND status = ANY(%s)"
)


class ClaimState(StrEnum):
    OWNED = "owned"  # this attempt still holds the live file
    LOST = "lost"  # the file is live, but another attempt took it over or finished it
    GONE = "gone"  # the file or its KB was deleted


@dataclass(frozen=True)
class IngestTarget:
    file_id: UUID
    org_id: str
    kb_id: UUID
    version: int
    name: str
    sha256: str
    external_parsing: bool
    collection_name: str
    embedding_model_id: UUID
    embedding_dims: int
    attempt_id: UUID


async def claim_file(conn: Connection, job: IngestJob, lease_seconds: float) -> IngestTarget | None:
    """None: deleted, superseded, finished, or leased by another delivery (ruling A10)."""
    row = await (
        await conn.execute(
            CLAIM,
            (
                lease_seconds,
                job.file_id,
                job.org_id,
                job.kb_id,
                job.version,
                [s.value for s in CLAIMABLE],
                [s.value for s in IN_PROGRESS],
            ),
        )
    ).fetchone()
    if row is None:
        return None
    return IngestTarget(
        file_id=row["id"],
        org_id=row["org_id"],
        kb_id=row["kb_id"],
        version=row["version"],
        name=row["name"],
        sha256=row["sha256"],
        external_parsing=row["external_parsing"],
        collection_name=row["collection_name"],
        embedding_model_id=row["embedding_model_id"],
        embedding_dims=row["embedding_dims"],
        attempt_id=row["ingest_attempt_id"],
    )


def _owned(target: IngestTarget) -> tuple[UUID, int, UUID, list[str]]:
    return (target.file_id, target.version, target.attempt_id, [s.value for s in IN_PROGRESS])


async def set_step(conn: Connection, target: IngestTarget, status: FileStatus) -> None:
    await conn.execute(
        b"UPDATE kb_files SET status = %s, updated_at = now() WHERE " + OWNED_BY_ATTEMPT,
        (status.value, *_owned(target)),
    )


async def claim_state(conn: Connection, target: IngestTarget) -> ClaimState:
    """Called under the KB lock, so no other attempt or purge can write this file's data until commit."""
    row = await (
        await conn.execute(
            b"SELECT f.ingest_attempt_id = %s AND f.status = ANY(%s) AS owned "
            b"FROM kb_files f JOIN knowledge_bases kb ON kb.id = f.kb_id "
            b"WHERE f.id = %s AND f.version = %s AND f.deleted_at IS NULL AND kb.deleted_at IS NULL",
            (target.attempt_id, [s.value for s in IN_PROGRESS], target.file_id, target.version),
        )
    ).fetchone()
    if row is None:
        return ClaimState.GONE
    return ClaimState.OWNED if row["owned"] else ClaimState.LOST


async def release_claim(conn: Connection, target: IngestTarget) -> None:
    """A failed attempt gives the file back so the stream's retry can claim it (ruling A24)."""
    await conn.execute(
        b"UPDATE kb_files SET status = 'pending', ingest_lease_until = NULL, updated_at = now() WHERE "
        + OWNED_BY_ATTEMPT,
        _owned(target),
    )


@dataclass(frozen=True)
class StalledFile:
    file_id: UUID
    org_id: str
    kb_id: UUID
    version: int
    recoveries: int


# In progress, but the lease ended more than the grace ago: the attempt is gone (crash, or its release failed)
# and the stream's own reclaim had its chance. A live attempt always ends before its lease (job deadline).
STALLED: Final = (
    b"WHERE deleted_at IS NULL AND status = ANY(%s) "
    b"AND ingest_lease_until < now() - make_interval(secs => %s) "
    # A tombstoned KB's files wait for its purge; re-driving them would only skip or dead-letter noise.
    b"AND NOT EXISTS (SELECT 1 FROM knowledge_bases kb "
    b"WHERE kb.id = kb_files.kb_id AND kb.deleted_at IS NOT NULL) "
)
STALLED_RETURNING: Final = b" RETURNING id, org_id, kb_id, version, ingest_recoveries"


def _stalled(rows: Sequence[DictRow]) -> list[StalledFile]:
    return [
        StalledFile(
            file_id=row["id"],
            org_id=row["org_id"],
            kb_id=row["kb_id"],
            version=row["version"],
            recoveries=row["ingest_recoveries"],
        )
        for row in rows
    ]


async def requeue_stalled(conn: Connection, grace_seconds: float, max_recoveries: int) -> list[StalledFile]:
    """Give stalled files back as pending; clearing the attempt id fences out any attempt still alive."""
    rows = await (
        await conn.execute(
            b"UPDATE kb_files SET status = 'pending', ingest_lease_until = NULL, ingest_attempt_id = NULL, "
            b"ingest_recoveries = ingest_recoveries + 1, updated_at = now() "
            + STALLED
            + b"AND ingest_recoveries < %s"
            + STALLED_RETURNING,
            ([s.value for s in IN_PROGRESS], grace_seconds, max_recoveries),
        )
    ).fetchall()
    return _stalled(rows)


async def fail_stalled(
    conn: Connection, grace_seconds: float, max_recoveries: int, error_code: str
) -> list[StalledFile]:
    """A file that vanished on every recovery is failed instead of re-driven forever (a poison file)."""
    rows = await (
        await conn.execute(
            b"UPDATE kb_files SET status = 'failed', error_code = %s, ingest_lease_until = NULL, "
            b"ingest_attempt_id = NULL, updated_at = now() "
            + STALLED
            + b"AND ingest_recoveries >= %s"
            + STALLED_RETURNING,
            (error_code, [s.value for s in IN_PROGRESS], grace_seconds, max_recoveries),
        )
    ).fetchall()
    return _stalled(rows)


async def replace_outline(
    conn: Connection, target: IngestTarget, outline: Outline, blocks: Sequence[Block]
) -> None:
    """Idempotent: a rerun (retry, replay, duplicate) replaces the previous parse of this file."""
    await conn.execute(
        b"DELETE FROM doc_nodes WHERE file_id = %s", (target.file_id,)
    )  # cascades blocks, tables
    async with conn.cursor() as cursor:
        await cursor.executemany(
            b"INSERT INTO doc_nodes (id, org_id, kb_id, file_id, parent_id, position, depth, title) "
            b"VALUES (%s, %s, %s, %s, %s, %s, %s, %s)",
            [
                (
                    n.node_id,
                    target.org_id,
                    target.kb_id,
                    target.file_id,
                    n.parent_id,
                    n.position,
                    n.depth,
                    n.title,
                )
                for n in outline.nodes
            ],
        )
        await cursor.executemany(
            b"INSERT INTO doc_blocks (file_id, block_id, org_id, kb_id, node_id, position, block_type, "
            b"text, locator) "
            b"VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)",
            [
                (
                    target.file_id,
                    b.block_id,
                    target.org_id,
                    target.kb_id,
                    outline.block_nodes[b.block_id],
                    position,
                    b.block_type.value,
                    b.text,
                    Jsonb(b.locator.to_json()),
                )
                for position, b in enumerate(blocks)
            ],
        )
        tables = [b for b in blocks if b.table is not None]
        await cursor.executemany(
            b"INSERT INTO kb_tables (id, org_id, kb_id, file_id, node_id, block_id, header, rows, locator) "
            b"VALUES (gen_random_uuid(), %s, %s, %s, %s, %s, %s, %s, %s)",
            [
                (
                    target.org_id,
                    target.kb_id,
                    target.file_id,
                    outline.block_nodes[b.block_id],
                    b.block_id,
                    Jsonb(list(b.table.header)),
                    Jsonb([list(r) for r in b.table.rows]),
                    Jsonb(b.locator.to_json()),
                )
                for b in tables
                if b.table is not None
            ],
        )


async def finish(conn: Connection, target: IngestTarget, parsed: ParsedDocument, chunk_count: int) -> bool:
    status = FileStatus.DEGRADED if parsed.degraded else FileStatus.READY
    row = await (
        await conn.execute(
            b"UPDATE kb_files SET status = %s, degraded = %s, parse_provider = %s, parse_quality = %s, "
            b"page_count = %s, chunk_count = %s, error_code = '', ingested_at = now(), "
            b"ingest_lease_until = NULL, ingest_recoveries = 0, "
            b"updated_at = now() "
            b"WHERE " + OWNED_BY_ATTEMPT + b" RETURNING id",
            (
                status.value,
                parsed.degraded,
                parsed.provider.value,
                parsed.quality,
                parsed.page_count,
                chunk_count,
                *_owned(target),
            ),
        )
    ).fetchone()
    return row is not None


async def set_summaries(conn: Connection, target: IngestTarget, summaries: Mapping[UUID, str]) -> None:
    if not summaries:
        return
    async with conn.cursor() as cursor:
        await cursor.executemany(
            b"UPDATE doc_nodes SET summary = %s WHERE id = %s AND file_id = %s "
            b"AND EXISTS (SELECT 1 FROM kb_files WHERE " + OWNED_BY_ATTEMPT + b")",
            [(text, node, target.file_id, *_owned(target)) for node, text in summaries.items()],
        )


async def supersede_older_versions(conn: Connection, target: IngestTarget, request_id: str) -> list[UUID]:
    """Same name, newer version ready: older versions leave the list and their points go (ruling B15)."""
    rows = await (
        await conn.execute(
            b"UPDATE kb_files SET deleted_at = now(), updated_at = now() "
            b"WHERE org_id = %s AND kb_id = %s AND name = %s AND version < %s AND deleted_at IS NULL "
            b"AND status <> 'uploading' RETURNING id",
            (target.org_id, target.kb_id, target.name, target.version),
        )
    ).fetchall()
    retired = [row["id"] for row in rows]
    for file_id in retired:
        payload = {"kind": PURGE_KIND_FILE, "kb_id": str(target.kb_id), "file_id": str(file_id)}
        await enqueue_outbox(conn, target.org_id, STREAM_KB_PURGE_REQUESTED, payload, request_id)
    return retired
