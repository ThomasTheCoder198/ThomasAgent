from datetime import datetime
from typing import Final
from uuid import UUID

from psycopg.rows import DictRow

from thomas_ragx.kb.repository import Connection

SEARCHABLE: Final = ["ready", "degraded"]
BEFORE_FIRST_POSITION: Final = -1  # block positions start at 0


async def searchable_file(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> DictRow | None:
    return await (
        await conn.execute(
            b"SELECT id, name FROM kb_files WHERE id = %s AND org_id = %s AND kb_id = %s AND "
            b"deleted_at IS NULL "
            b"AND status = ANY(%s)",
            (file_id, org_id, kb_id, SEARCHABLE),
        )
    ).fetchone()


async def outline_nodes(
    conn: Connection, org_id: str, kb_id: UUID, file_id: UUID, limit: int
) -> list[DictRow]:
    return await (
        await conn.execute(
            b"SELECT id, parent_id, depth, title, summary FROM doc_nodes WHERE org_id = %s AND "
            b"kb_id = %s AND file_id = %s ORDER BY position LIMIT %s",
            (org_id, kb_id, file_id, limit),
        )
    ).fetchall()


async def node_in_kb(conn: Connection, org_id: str, kb_id: UUID, node_id: UUID) -> DictRow | None:
    return await (
        await conn.execute(
            b"SELECT n.id, n.file_id, n.title, f.name AS file_name FROM doc_nodes n JOIN kb_files "
            b"f ON f.id = n.file_id "
            b"WHERE n.id = %s AND n.org_id = %s AND n.kb_id = %s AND f.deleted_at IS NULL AND "
            b"f.status = ANY(%s)",
            (node_id, org_id, kb_id, SEARCHABLE),
        )
    ).fetchone()


async def node_path(conn: Connection, org_id: str, kb_id: UUID, node_id: UUID) -> list[str]:
    rows = await (
        await conn.execute(
            b"WITH RECURSIVE up AS (SELECT id, parent_id, title, depth FROM doc_nodes WHERE org_id "
            b"= %s AND kb_id = %s AND id = %s "
            b"UNION ALL SELECT n.id, n.parent_id, n.title, n.depth FROM doc_nodes n JOIN up ON "
            b"n.id = up.parent_id WHERE n.org_id = %s AND n.kb_id = %s) "
            b"SELECT title FROM up WHERE depth > 0 ORDER BY depth",
            (org_id, kb_id, node_id, org_id, kb_id),
        )
    ).fetchall()
    return [row["title"] for row in rows]


async def section_blocks(
    conn: Connection,
    org_id: str,
    kb_id: UUID,
    file_id: UUID,
    node_id: UUID,
    *,
    after_position: int = BEFORE_FIRST_POSITION,
    limit: int,
) -> list[DictRow]:
    """One page of a section's blocks in document order; the caller pages on `position` (Codex I2)."""
    return await (
        await conn.execute(
            b"WITH RECURSIVE tree AS (SELECT id FROM doc_nodes WHERE org_id = %s AND kb_id = %s AND id = %s "
            b"UNION ALL SELECT n.id FROM doc_nodes n JOIN tree t ON n.parent_id = t.id WHERE "
            b"n.org_id = %s AND n.kb_id = %s) "
            b"SELECT block_id, block_type, text, locator, position FROM doc_blocks "
            b"WHERE org_id = %s AND kb_id = %s AND file_id = %s AND node_id IN (SELECT id FROM "
            b"tree) AND position > %s ORDER BY position LIMIT %s",
            (org_id, kb_id, node_id, org_id, kb_id, org_id, kb_id, file_id, after_position, limit),
        )
    ).fetchall()


async def list_documents(
    conn: Connection,
    org_id: str,
    kb_id: UUID,
    name_contains: str | None,
    suffix: str | None,
    after: tuple[datetime, UUID] | None,
    limit: int,
) -> list[DictRow]:
    after_at, after_id = after if after is not None else (None, None)
    return await (
        await conn.execute(
            b"SELECT id, name, version, status, page_count, created_at FROM kb_files "
            b"WHERE org_id = %s AND kb_id = %s AND deleted_at IS NULL AND status <> 'uploading' "
            b"AND (%s::text IS NULL OR position(lower(%s::text) in lower(name)) > 0) "
            b"AND (%s::text IS NULL OR lower(name) LIKE '%%' || %s::text) "
            b"AND (%s::timestamptz IS NULL OR (created_at, id) < (%s::timestamptz, %s::uuid)) "
            b"ORDER BY created_at DESC, id DESC LIMIT %s",
            (
                org_id,
                kb_id,
                name_contains,
                name_contains,
                suffix,
                suffix,
                after_at,
                after_at,
                after_id,
                limit,
            ),
        )
    ).fetchall()
