from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final
from uuid import UUID

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
    b"ingest_lease_until = now() + make_interval(secs => %s) "
    b"FROM knowledge_bases kb, orgs o "
    b"WHERE f.id = %s AND f.org_id = %s AND f.kb_id = %s AND f.version = %s AND f.deleted_at IS NULL "
    b"AND kb.id = f.kb_id AND kb.deleted_at IS NULL AND o.id = f.org_id "
    b"AND (f.status = ANY(%s) OR (f.status = ANY(%s) AND f.ingest_lease_until < now())) "
    b"RETURNING f.id, f.org_id, f.kb_id, f.version, f.name, f.sha256, o.external_parsing, "
    b"kb.collection_name, kb.embedding_model_id, kb.embedding_dims"
)


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
    )


async def set_step(conn: Connection, file_id: UUID, status: FileStatus) -> None:
    await conn.execute(
        b"UPDATE kb_files SET status = %s, updated_at = now() "
        b"WHERE id = %s AND deleted_at IS NULL AND status = ANY(%s)",
        (status.value, file_id, [s.value for s in IN_PROGRESS]),
    )


async def is_live(conn: Connection, file_id: UUID, version: int) -> bool:
    row = await (
        await conn.execute(
            b"SELECT 1 FROM kb_files f JOIN knowledge_bases kb ON kb.id = f.kb_id "
            b"WHERE f.id = %s AND f.version = %s AND f.deleted_at IS NULL AND kb.deleted_at IS NULL",
            (file_id, version),
        )
    ).fetchone()
    return row is not None


async def release_claim(conn: Connection, file_id: UUID, version: int) -> None:
    """A failed attempt gives the file back so the stream's retry can claim it (ruling A24)."""
    await conn.execute(
        b"UPDATE kb_files SET status = 'pending', ingest_lease_until = NULL, updated_at = now() "
        b"WHERE id = %s AND version = %s AND deleted_at IS NULL AND status = ANY(%s)",
        (file_id, version, [s.value for s in IN_PROGRESS]),
    )


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
            b"ingest_lease_until = NULL, "
            b"updated_at = now() "
            b"WHERE id = %s AND version = %s AND deleted_at IS NULL AND status = ANY(%s) RETURNING id",
            (
                status.value,
                parsed.degraded,
                parsed.provider.value,
                parsed.quality,
                parsed.page_count,
                chunk_count,
                target.file_id,
                target.version,
                [s.value for s in IN_PROGRESS],
            ),
        )
    ).fetchone()
    return row is not None


async def set_summaries(conn: Connection, summaries: Mapping[UUID, str]) -> None:
    if not summaries:
        return
    async with conn.cursor() as cursor:
        await cursor.executemany(
            b"UPDATE doc_nodes SET summary = %s WHERE id = %s",
            [(text, node) for node, text in summaries.items()],
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
