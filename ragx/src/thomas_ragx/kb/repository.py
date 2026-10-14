from typing import Final
from uuid import UUID

from psycopg import AsyncConnection
from psycopg.rows import DictRow

from thomas_ragx.kb.model import KnowledgeBase, Org, collection_name_for
from thomas_ragx.registry.model import ModelRecord

type Connection = AsyncConnection[DictRow]
KB_SELECT: Final = (
    b"SELECT kb.id, kb.org_id, kb.name, kb.is_default, kb.embedding_dims, kb.embedding_model_id, "
    b"kb.collection_name, "
    b"kb.created_at, kb.updated_at, "
    b"m.name AS embedding_model, "
    b"(SELECT count(*) FROM kb_files f WHERE f.kb_id = kb.id AND f.deleted_at IS NULL "
    b"AND f.status <> 'uploading') AS file_count "
    b"FROM knowledge_bases kb JOIN models m ON m.id = kb.embedding_model_id "
    b"WHERE kb.deleted_at IS NULL AND kb.org_id = %s"
)


def to_kb(row: DictRow) -> KnowledgeBase:
    return KnowledgeBase(
        id=row["id"],
        org_id=row["org_id"],
        name=row["name"],
        is_default=row["is_default"],
        embedding_model=row["embedding_model"],
        embedding_model_id=row["embedding_model_id"],
        embedding_dims=row["embedding_dims"],
        collection_name=row["collection_name"],
        file_count=row["file_count"],
        created_at=row["created_at"],
        updated_at=row["updated_at"],
    )


async def upsert_org(conn: Connection, org_id: str, name: str, external_parsing: bool) -> None:
    await conn.execute(
        b"INSERT INTO orgs (id, name, external_parsing) VALUES (%s, %s, %s) ON CONFLICT (id) DO UPDATE "
        b"SET name = EXCLUDED.name, external_parsing = EXCLUDED.external_parsing, updated_at = now()",
        (org_id, name, external_parsing),
    )


async def lock_org(conn: Connection, org_id: str) -> DictRow | None:
    return await (await conn.execute(b"SELECT * FROM orgs WHERE id = %s FOR UPDATE", (org_id,))).fetchone()


async def org_exists(conn: Connection, org_id: str) -> bool:
    return await (await conn.execute(b"SELECT 1 FROM orgs WHERE id = %s", (org_id,))).fetchone() is not None


async def default_kb_id(conn: Connection, org_id: str) -> UUID | None:
    row = await (
        await conn.execute(b"SELECT id FROM knowledge_bases WHERE org_id = %s AND is_default", (org_id,))
    ).fetchone()
    return row["id"] if row else None


async def insert_kb(
    conn: Connection, kb_id: UUID, org_id: str, name: str, is_default: bool, model: ModelRecord
) -> None:
    if model.embedding_dims is None:
        # The registry refuses an embedding role without dims, so this guards a broken caller.
        raise ValueError("an embedding model without dims cannot back a KB")
    collection = collection_name_for(model.id, model.embedding_dims)
    await conn.execute(
        b"INSERT INTO knowledge_bases "
        b"(id, org_id, name, is_default, embedding_model_id, embedding_dims, collection_name) "
        b"VALUES (%s, %s, %s, %s, %s, %s, %s)",
        (kb_id, org_id, name, is_default, model.id, model.embedding_dims, collection),
    )


async def get_kb(conn: Connection, org_id: str, kb_id: UUID) -> KnowledgeBase | None:
    row = await (await conn.execute(KB_SELECT + b" AND kb.id = %s", (org_id, kb_id))).fetchone()
    return to_kb(row) if row else None


async def lock_kb(conn: Connection, org_id: str, kb_id: UUID) -> KnowledgeBase | None:
    row = await (
        await conn.execute(KB_SELECT + b" AND kb.id = %s FOR UPDATE OF kb", (org_id, kb_id))
    ).fetchone()
    return to_kb(row) if row else None


async def list_kbs(conn: Connection, org_id: str) -> list[KnowledgeBase]:
    rows = await (
        await conn.execute(KB_SELECT + b" ORDER BY kb.is_default DESC, kb.name", (org_id,))
    ).fetchall()
    return [to_kb(row) for row in rows]


async def rename_kb(conn: Connection, org_id: str, kb_id: UUID, name: str) -> bool:
    cursor = await conn.execute(
        b"UPDATE knowledge_bases SET name = %s, updated_at = now() "
        b"WHERE id = %s AND org_id = %s AND deleted_at IS NULL",
        (name, kb_id, org_id),
    )
    return cursor.rowcount > 0


async def mark_kb_deleted(conn: Connection, org_id: str, kb_id: UUID) -> None:
    await conn.execute(
        b"UPDATE knowledge_bases SET deleted_at = now(), updated_at = now() WHERE id = %s AND org_id = %s",
        (kb_id, org_id),
    )


def to_org(row: DictRow, default_kb: UUID) -> Org:
    return Org(
        id=row["id"],
        name=row["name"],
        external_parsing=row["external_parsing"],
        default_kb_id=default_kb,
        created_at=row["created_at"],
    )


async def lock_collection(conn: Connection, org_id: str, kb_id: UUID) -> str | None:
    """Include tombstones so publication and purge hold the same lock until external writes finish."""
    row = await (
        await conn.execute(
            b"SELECT collection_name FROM knowledge_bases WHERE id = %s AND org_id = %s FOR UPDATE",
            (kb_id, org_id),
        )
    ).fetchone()
    return row["collection_name"] if row is not None else None


async def delete_kb_row(conn: Connection, org_id: str, kb_id: UUID) -> None:
    await conn.execute(
        b"DELETE FROM knowledge_bases WHERE id = %s AND org_id = %s AND deleted_at IS NOT NULL",
        (kb_id, org_id),
    )
