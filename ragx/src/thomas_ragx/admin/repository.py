import uuid
from datetime import datetime
from uuid import UUID

from psycopg import AsyncConnection
from psycopg.rows import DictRow

from thomas_ragx.admin.model import AdminSession

type Connection = AsyncConnection[DictRow]


async def insert_admin(conn: Connection, email: str, password_hash: str) -> bool:
    cursor = await conn.execute(
        b"INSERT INTO admin_users (id, email, password_hash) VALUES (%s, %s, %s) "
        b"ON CONFLICT (email) DO NOTHING",
        (uuid.uuid4(), email, password_hash),
    )
    return cursor.rowcount > 0


async def admin_by_email(conn: Connection, email: str) -> DictRow | None:
    return await (await conn.execute(b"SELECT * FROM admin_users WHERE email = %s", (email,))).fetchone()


async def insert_session(
    conn: Connection, token_hash: bytes, admin_user_id: UUID, csrf_token: str, expires_at: datetime
) -> None:
    await conn.execute(b"DELETE FROM ui_sessions WHERE expires_at <= now()")
    await conn.execute(
        b"INSERT INTO ui_sessions (token_hash, admin_user_id, csrf_token, expires_at) "
        b"VALUES (%s, %s, %s, %s)",
        (token_hash, admin_user_id, csrf_token, expires_at),
    )


async def session_by_hash(conn: Connection, token_hash: bytes) -> AdminSession | None:
    row = await (
        await conn.execute(
            b"SELECT s.admin_user_id, a.email, s.csrf_token, s.expires_at FROM ui_sessions s "
            b"JOIN admin_users a ON a.id = s.admin_user_id WHERE s.token_hash = %s AND s.expires_at > now()",
            (token_hash,),
        )
    ).fetchone()
    if row is None:
        return None
    return AdminSession(row["admin_user_id"], row["email"], row["csrf_token"], row["expires_at"])


async def delete_session(conn: Connection, token_hash: bytes) -> None:
    await conn.execute(b"DELETE FROM ui_sessions WHERE token_hash = %s", (token_hash,))
