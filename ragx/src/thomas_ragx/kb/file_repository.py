from datetime import datetime
from typing import Final
from uuid import UUID

from psycopg.rows import DictRow

from thomas_ragx.kb.model import FileStatus, KbFile, UploadSlot
from thomas_ragx.kb.repository import Connection

OPERATION_UPLOAD: Final = "upload"
OPERATION_CONFIRM: Final = "confirm"
# An 'uploading' slot is invisible to every read: only confirm sees it, through the slot functions below.
FILE_SELECT: Final = b"SELECT * FROM kb_files WHERE deleted_at IS NULL AND status <> 'uploading'"
SLOT_SELECT: Final = (
    b"SELECT id, org_id, kb_id, name, content_type, size_bytes, created_at FROM kb_files "
    b"WHERE org_id = %s AND kb_id = %s AND id = %s AND status = 'uploading' AND deleted_at IS NULL"
)


def to_file(row: DictRow) -> KbFile:
    return KbFile(
        id=row["id"],
        org_id=row["org_id"],
        kb_id=row["kb_id"],
        name=row["name"],
        version=row["version"],
        content_type=row["content_type"],
        size_bytes=row["size_bytes"],
        sha256=row["sha256"],
        status=FileStatus(row["status"]),
        error_code=row["error_code"],
        degraded=row["degraded"],
        created_at=row["created_at"],
        updated_at=row["updated_at"],
    )


async def find_by_sha(conn: Connection, kb_id: UUID, sha256: str) -> KbFile | None:
    row = await (
        await conn.execute(FILE_SELECT + b" AND kb_id = %s AND sha256 = %s", (kb_id, sha256))
    ).fetchone()
    return to_file(row) if row else None


async def next_version(conn: Connection, kb_id: UUID, name: str) -> int:
    row = await (
        await conn.execute(
            b"SELECT COALESCE(MAX(version), 0) + 1 AS version FROM kb_files WHERE kb_id = %s AND name = %s",
            (kb_id, name),
        )
    ).fetchone()
    return int(row["version"]) if row else 1


async def insert_upload_slot(
    conn: Connection, file_id: UUID, org_id: str, kb_id: UUID, name: str, content_type: str, size_bytes: int
) -> None:
    await conn.execute(
        b"INSERT INTO kb_files (id, org_id, kb_id, name, content_type, size_bytes, status) "
        b"VALUES (%s, %s, %s, %s, %s, %s, 'uploading')",
        (file_id, org_id, kb_id, name, content_type, size_bytes),
    )


def _to_slot(row: DictRow) -> UploadSlot:
    return UploadSlot(
        id=row["id"],
        org_id=row["org_id"],
        kb_id=row["kb_id"],
        name=row["name"],
        content_type=row["content_type"],
        size_bytes=row["size_bytes"],
        created_at=row["created_at"],
    )


async def get_upload_slot(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> UploadSlot | None:
    row = await (await conn.execute(SLOT_SELECT, (org_id, kb_id, file_id))).fetchone()
    return _to_slot(row) if row else None


async def lock_upload_slot(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> UploadSlot | None:
    row = await (await conn.execute(SLOT_SELECT + b" FOR UPDATE", (org_id, kb_id, file_id))).fetchone()
    return _to_slot(row) if row else None


async def confirm_upload_slot(conn: Connection, file_id: UUID, sha256: str, version: int) -> KbFile:
    row = await (
        await conn.execute(
            b"UPDATE kb_files SET status = 'pending', sha256 = %s, version = %s, updated_at = now() "
            b"WHERE id = %s AND status = 'uploading' RETURNING *",
            (sha256, version, file_id),
        )
    ).fetchone()
    if row is None:
        raise RuntimeError("UPDATE … RETURNING returned no row")
    return to_file(row)


async def delete_upload_slot(conn: Connection, file_id: UUID) -> bool:
    """False when the slot is gone or was confirmed meanwhile: the DELETE waits on confirm's row lock."""
    cursor = await conn.execute(b"DELETE FROM kb_files WHERE id = %s AND status = 'uploading'", (file_id,))
    return cursor.rowcount > 0


async def expired_upload_slots(conn: Connection, older_than_seconds: float) -> list[UploadSlot]:
    # The age is judged by the database clock, the one that stamped created_at.
    rows = await (
        await conn.execute(
            b"SELECT id, org_id, kb_id, name, content_type, size_bytes, created_at FROM kb_files "
            b"WHERE status = 'uploading' AND created_at < now() - make_interval(secs => %s)",
            (older_than_seconds,),
        )
    ).fetchall()
    return [_to_slot(row) for row in rows]


async def existing_file_ids(conn: Connection, ids: list[UUID]) -> set[UUID]:
    rows = await (await conn.execute(b"SELECT id FROM kb_files WHERE id = ANY(%s)", (ids,))).fetchall()
    return {row["id"] for row in rows}


async def mark_ingest_failed(conn: Connection, org_id: str, file_id: UUID, error_code: str) -> bool:
    """False when the file already finished (or is gone) and the row was left alone."""
    cursor = await conn.execute(
        b"UPDATE kb_files SET status = 'failed', error_code = %s, ingest_lease_until = NULL, "
        b"updated_at = now() WHERE id = %s AND org_id = %s AND deleted_at IS NULL "
        b"AND status NOT IN ('uploading', 'ready', 'degraded')",
        (error_code, file_id, org_id),
    )
    return cursor.rowcount > 0


async def get_file(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> KbFile | None:
    row = await (
        await conn.execute(
            FILE_SELECT + b" AND org_id = %s AND kb_id = %s AND id = %s", (org_id, kb_id, file_id)
        )
    ).fetchone()
    return to_file(row) if row else None


async def list_files(
    conn: Connection, org_id: str, kb_id: UUID, after: tuple[datetime, UUID] | None, limit: int
) -> list[KbFile]:
    if after is None:
        cursor = await conn.execute(
            FILE_SELECT + b" AND org_id = %s AND kb_id = %s ORDER BY created_at DESC, id DESC LIMIT %s",
            (org_id, kb_id, limit),
        )
    else:
        cursor = await conn.execute(
            FILE_SELECT + b" AND org_id = %s AND kb_id = %s AND (created_at, id) < (%s, %s) "
            b"ORDER BY created_at DESC, id DESC LIMIT %s",
            (org_id, kb_id, after[0], after[1], limit),
        )
    return [to_file(row) for row in await cursor.fetchall()]


async def mark_file_deleted(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> bool:
    cursor = await conn.execute(
        b"UPDATE kb_files SET deleted_at = now(), updated_at = now() "
        b"WHERE id = %s AND org_id = %s AND kb_id = %s AND deleted_at IS NULL AND status <> 'uploading'",
        (file_id, org_id, kb_id),
    )
    return cursor.rowcount > 0


async def requeue_failed(conn: Connection, org_id: str, kb_id: UUID, file_id: UUID) -> KbFile | None:
    row = await (
        await conn.execute(
            b"UPDATE kb_files SET status = 'pending', error_code = '', updated_at = now() "
            b"WHERE id = %s AND org_id = %s AND kb_id = %s AND deleted_at IS NULL "
            b"AND status = 'failed' RETURNING *",
            (file_id, org_id, kb_id),
        )
    ).fetchone()
    return to_file(row) if row else None


async def remembered_file_id(
    conn: Connection, org_id: str, key: str, ttl_seconds: int, *, operation: str = OPERATION_UPLOAD
) -> UUID | None:
    await conn.execute(
        b"DELETE FROM idempotency_keys WHERE org_id = %s AND operation = %s AND key = %s "
        b"AND created_at <= now() - make_interval(secs => %s)",
        (org_id, operation, key, ttl_seconds),
    )
    row = await (
        await conn.execute(
            b"SELECT file_id FROM idempotency_keys WHERE org_id = %s AND operation = %s AND key = %s",
            (org_id, operation, key),
        )
    ).fetchone()
    return row["file_id"] if row else None


async def remember_file_id(
    conn: Connection, org_id: str, key: str, file_id: UUID, *, operation: str = OPERATION_UPLOAD
) -> bool:
    row = await (
        await conn.execute(
            b"INSERT INTO idempotency_keys (org_id, operation, key, file_id) VALUES (%s, %s, %s, %s) "
            b"ON CONFLICT DO NOTHING RETURNING file_id",
            (org_id, operation, key, file_id),
        )
    ).fetchone()
    return row is not None


async def delete_file_row(conn: Connection, org_id: str, file_id: UUID) -> None:
    await conn.execute(
        b"DELETE FROM kb_files WHERE id = %s AND org_id = %s AND deleted_at IS NOT NULL", (file_id, org_id)
    )
