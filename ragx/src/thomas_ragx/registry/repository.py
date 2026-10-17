from collections.abc import Iterable
from typing import Any, Final
from uuid import UUID

from psycopg import AsyncConnection
from psycopg.rows import DictRow

from thomas_ragx.registry.model import (
    Capability,
    ModelFacts,
    ModelRecord,
    ModelSource,
    Provider,
    ProviderKind,
    Role,
)
from thomas_ragx.registry.vault import Sealed

type Connection = AsyncConnection[DictRow]
MODEL_COLUMNS: Final = (
    b"id, provider_id, name, capabilities, capabilities_edited, embedding_dims, context_window, "
    b"input_cost_per_token, output_cost_per_token, source"
)


def to_provider(row: DictRow) -> Provider:
    return Provider(
        id=row["id"],
        kind=ProviderKind(row["kind"]),
        name=row["name"],
        base_url=row["base_url"],
        has_api_key=row["api_key_ciphertext"] is not None,
    )


def to_model(row: DictRow) -> ModelRecord:
    return ModelRecord(
        id=row["id"],
        provider_id=row["provider_id"],
        name=row["name"],
        capabilities=frozenset(Capability(value) for value in row["capabilities"]),
        capabilities_edited=row["capabilities_edited"],
        embedding_dims=row["embedding_dims"],
        context_window=row["context_window"],
        input_cost_per_token=row["input_cost_per_token"],
        output_cost_per_token=row["output_cost_per_token"],
        source=ModelSource(row["source"]),
    )


def _capability_values(capabilities: Iterable[Capability]) -> list[str]:
    return sorted(capability.value for capability in capabilities)


async def _one(conn: Connection, query: bytes, params: tuple[Any, ...]) -> DictRow | None:
    return await (await conn.execute(query, params)).fetchone()


async def insert_provider(
    conn: Connection, provider_id: UUID, kind: ProviderKind, name: str, base_url: str, sealed: Sealed | None
) -> Provider:
    row = await _one(
        conn,
        b"INSERT INTO providers (id, kind, name, base_url, api_key_nonce, api_key_ciphertext) "
        b"VALUES (%s, %s, %s, %s, %s, %s) RETURNING *",
        (
            provider_id,
            kind.value,
            name,
            base_url,
            sealed.nonce if sealed else None,
            sealed.ciphertext if sealed else None,
        ),
    )
    assert row is not None  # noqa: S101 - INSERT … RETURNING always yields the row
    return to_provider(row)


async def update_provider(conn: Connection, provider_id: UUID, name: str, base_url: str) -> Provider | None:
    row = await _one(
        conn,
        b"UPDATE providers SET name = %s, base_url = %s, updated_at = now() WHERE id = %s RETURNING *",
        (name, base_url, provider_id),
    )
    return to_provider(row) if row else None


async def set_provider_key(conn: Connection, provider_id: UUID, sealed: Sealed | None) -> None:
    await conn.execute(
        b"UPDATE providers SET api_key_nonce = %s, api_key_ciphertext = %s, updated_at = now() WHERE id = %s",
        (sealed.nonce if sealed else None, sealed.ciphertext if sealed else None, provider_id),
    )


async def get_provider_row(conn: Connection, provider_id: UUID) -> DictRow | None:
    return await _one(conn, b"SELECT * FROM providers WHERE id = %s", (provider_id,))


async def get_provider_row_for_update(conn: Connection, provider_id: UUID) -> DictRow | None:
    return await _one(conn, b"SELECT * FROM providers WHERE id = %s FOR UPDATE", (provider_id,))


async def get_provider_by_name(conn: Connection, name: str) -> Provider | None:
    row = await _one(conn, b"SELECT * FROM providers WHERE name = %s", (name,))
    return to_provider(row) if row else None


async def list_providers(conn: Connection) -> list[Provider]:
    rows = await (await conn.execute(b"SELECT * FROM providers ORDER BY name")).fetchall()
    return [to_provider(row) for row in rows]


async def delete_provider(conn: Connection, provider_id: UUID) -> bool:
    await conn.execute(b"DELETE FROM models WHERE provider_id = %s", (provider_id,))
    deleted = await conn.execute(b"DELETE FROM providers WHERE id = %s", (provider_id,))
    return deleted.rowcount > 0


async def upsert_synced_model(
    conn: Connection, model_id: UUID, provider_id: UUID, name: str, facts: ModelFacts
) -> None:
    # Manual rows and operator-edited capabilities are never overwritten by a sync. Dims only fill a gap:
    # most lists carry none, and a measured value (or a KB's collection size) must survive a re-sync.
    await conn.execute(
        b"INSERT INTO models (id, provider_id, name, capabilities, embedding_dims, context_window, "
        b"input_cost_per_token, output_cost_per_token, source) "
        b"VALUES (%s, %s, %s, %s, %s, %s, %s, %s, 'sync') "
        b"ON CONFLICT (provider_id, name) DO UPDATE SET "
        b"capabilities = CASE WHEN models.capabilities_edited THEN models.capabilities "
        b"ELSE EXCLUDED.capabilities END, "
        b"embedding_dims = CASE WHEN models.capabilities_edited OR EXISTS ("
        b"SELECT 1 FROM knowledge_bases kb WHERE kb.embedding_model_id = models.id) "
        b"THEN models.embedding_dims ELSE COALESCE(models.embedding_dims, EXCLUDED.embedding_dims) END, "
        b"context_window = EXCLUDED.context_window, input_cost_per_token = EXCLUDED.input_cost_per_token, "
        b"output_cost_per_token = EXCLUDED.output_cost_per_token, updated_at = now() "
        b"WHERE models.source = 'sync'",
        (
            model_id,
            provider_id,
            name,
            _capability_values(facts.capabilities),
            facts.embedding_dims,
            facts.context_window,
            facts.input_cost_per_token,
            facts.output_cost_per_token,
        ),
    )


async def insert_manual_model(
    conn: Connection,
    model_id: UUID,
    provider_id: UUID,
    name: str,
    capabilities: frozenset[Capability],
    embedding_dims: int | None,
    context_window: int | None,
) -> ModelRecord:
    row = await _one(
        conn,
        b"INSERT INTO models (id, provider_id, name, capabilities, capabilities_edited, embedding_dims, "
        b"context_window, source) VALUES (%s, %s, %s, %s, true, %s, %s, 'manual') RETURNING " + MODEL_COLUMNS,
        (model_id, provider_id, name, _capability_values(capabilities), embedding_dims, context_window),
    )
    assert row is not None  # noqa: S101 - INSERT … RETURNING always yields the row
    return to_model(row)


async def update_model(
    conn: Connection, model_id: UUID, capabilities: frozenset[Capability]
) -> ModelRecord | None:
    # Dims are not editable here: only the measured path, a manual add and an empty-column sync write them.
    row = await _one(
        conn,
        b"UPDATE models SET capabilities = %s, capabilities_edited = true, updated_at = now() "
        b"WHERE id = %s RETURNING " + MODEL_COLUMNS,
        (_capability_values(capabilities), model_id),
    )
    return to_model(row) if row else None


async def set_measured_dims(conn: Connection, model_id: UUID, dims: int) -> bool:
    """Fills missing dims only: a KB copies them into its collection, so a used model's size never moves.

    Measuring is not an operator edit, so `capabilities_edited` stays as it was.
    """
    result = await conn.execute(
        b"UPDATE models SET embedding_dims = %s, updated_at = now() WHERE id = %s AND embedding_dims IS NULL "
        b"AND NOT EXISTS (SELECT 1 FROM knowledge_bases kb WHERE kb.embedding_model_id = models.id)",
        (dims, model_id),
    )
    return result.rowcount > 0


async def get_model(conn: Connection, model_id: UUID) -> ModelRecord | None:
    row = await _one(conn, b"SELECT " + MODEL_COLUMNS + b" FROM models WHERE id = %s", (model_id,))
    return to_model(row) if row else None


async def lock_model(conn: Connection, model_id: UUID) -> ModelRecord | None:
    row = await _one(conn, b"SELECT " + MODEL_COLUMNS + b" FROM models WHERE id = %s FOR UPDATE", (model_id,))
    return to_model(row) if row else None


async def get_model_by_name(conn: Connection, provider_id: UUID, name: str) -> ModelRecord | None:
    row = await _one(
        conn,
        b"SELECT " + MODEL_COLUMNS + b" FROM models WHERE provider_id = %s AND name = %s",
        (provider_id, name),
    )
    return to_model(row) if row else None


async def list_models(conn: Connection, provider_id: UUID | None) -> list[ModelRecord]:
    if provider_id is None:
        cursor = await conn.execute(b"SELECT " + MODEL_COLUMNS + b" FROM models ORDER BY name")
    else:
        cursor = await conn.execute(
            b"SELECT " + MODEL_COLUMNS + b" FROM models WHERE provider_id = %s ORDER BY name", (provider_id,)
        )
    return [to_model(row) for row in await cursor.fetchall()]


async def delete_model(conn: Connection, model_id: UUID) -> bool:
    return (await conn.execute(b"DELETE FROM models WHERE id = %s", (model_id,))).rowcount > 0


async def assign_role(conn: Connection, role: Role, model_id: UUID) -> None:
    await conn.execute(
        b"INSERT INTO model_roles (role, model_id) VALUES (%s, %s) "
        b"ON CONFLICT (role) DO UPDATE SET model_id = EXCLUDED.model_id, updated_at = now()",
        (role.value, model_id),
    )


async def clear_role(conn: Connection, role: Role) -> None:
    await conn.execute(b"DELETE FROM model_roles WHERE role = %s", (role.value,))


async def role_assignments(conn: Connection) -> dict[Role, ModelRecord]:
    rows = await (
        await conn.execute(
            b"SELECT r.role, m.id, m.provider_id, m.name, m.capabilities, "
            b"m.capabilities_edited, m.embedding_dims, "
            b"m.context_window, m.input_cost_per_token, m.output_cost_per_token, m.source "
            b"FROM model_roles r JOIN models m ON m.id = r.model_id"
        )
    ).fetchall()
    return {Role(row["role"]): to_model(row) for row in rows}
