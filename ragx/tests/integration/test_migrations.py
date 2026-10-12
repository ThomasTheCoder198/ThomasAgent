import uuid

import psycopg
import pytest

from thomas_ragx.db.migrate import load_migrations, migrate_down, migrate_up

pytestmark = pytest.mark.integration


def _fresh_database(postgres_url: str, name: str) -> str:
    with psycopg.connect(postgres_url, autocommit=True) as conn:
        conn.execute(f'DROP DATABASE IF EXISTS "{name}"'.encode())
        conn.execute(f'CREATE DATABASE "{name}"'.encode())
    return postgres_url.rsplit("/", 1)[0] + "/" + name


def _tables(url: str) -> set[str]:
    with psycopg.connect(url) as conn:
        rows = conn.execute(b"SELECT tablename FROM pg_tables WHERE schemaname = 'public'").fetchall()
    return {row[0] for row in rows}


def test_migrations_are_paired_and_ordered() -> None:
    migrations = load_migrations()
    assert [m.version for m in migrations] == sorted({m.version for m in migrations})
    assert all(m.up and m.down for m in migrations)


def test_migrate_up_then_down_leaves_only_the_bookkeeping_table(postgres_url: str) -> None:
    url = _fresh_database(postgres_url, "migrate_check")
    applied = migrate_up(url)
    assert applied == [m.version for m in load_migrations()]
    assert {
        "orgs",
        "providers",
        "models",
        "model_roles",
        "knowledge_bases",
        "kb_files",
        "outbox",
        "doc_nodes",
        "doc_blocks",
        "kb_tables",
        "embedding_cache",
    } <= _tables(url)
    assert migrate_up(url) == []
    reverted = migrate_down(url, steps=len(applied))
    assert reverted == list(reversed(applied))
    assert _tables(url) == {"schema_migrations"}
    assert migrate_up(url) == applied


def test_presigned_uploads_down_removes_uploading_rows(postgres_url: str) -> None:
    url = _fresh_database(postgres_url, "migrate_uploading")
    migrate_up(url)
    kb_id = uuid.uuid4()
    with psycopg.connect(url, autocommit=True) as conn:
        provider_id, model_id = uuid.uuid4(), uuid.uuid4()
        conn.execute(b"INSERT INTO orgs (id, name, external_parsing) VALUES ('o', 'O', false)")
        conn.execute(b"INSERT INTO providers (id, kind, name) VALUES (%s, 'openai', 'p')", (provider_id,))
        conn.execute(
            b"INSERT INTO models (id, provider_id, name, embedding_dims, source) "
            b"VALUES (%s, %s, 'e', 8, 'manual')",
            (model_id, provider_id),
        )
        conn.execute(
            b"INSERT INTO knowledge_bases "
            b"(id, org_id, name, is_default, embedding_model_id, embedding_dims, collection_name) "
            b"VALUES (%s, 'o', 'kb', true, %s, 8, 'kb_x_8')",
            (kb_id, model_id),
        )
        conn.execute(
            b"INSERT INTO kb_files (id, org_id, kb_id, name, content_type, size_bytes, status) "
            b"VALUES (gen_random_uuid(), 'o', %s, 'a.txt', 'text/plain', 1, 'uploading')",
            (kb_id,),
        )
        with pytest.raises(psycopg.errors.CheckViolation):
            conn.execute(
                b"INSERT INTO kb_files (id, org_id, kb_id, name, content_type, size_bytes, status) "
                b"VALUES (gen_random_uuid(), 'o', %s, 'b.txt', 'text/plain', 1, 'pending')",
                (kb_id,),
            )
    assert migrate_down(url, steps=2) == [4, 3]
    with psycopg.connect(url) as conn:
        assert conn.execute(b"SELECT count(*) FROM kb_files").fetchone() == (0,)
    assert migrate_up(url) == [3, 4]


def test_rag_pipeline_up_backfills_collection_names_and_down_drops_the_tables(postgres_url: str) -> None:
    url = _fresh_database(postgres_url, "migrate_pipeline")
    migrate_up(url)
    assert migrate_down(url, steps=1) == [4]
    provider_id, model_id, kb_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    with psycopg.connect(url, autocommit=True) as conn:
        conn.execute(b"INSERT INTO orgs (id, name, external_parsing) VALUES ('o', 'O', true)")
        conn.execute(b"INSERT INTO providers (id, kind, name) VALUES (%s, 'openai', 'p')", (provider_id,))
        conn.execute(
            b"INSERT INTO models (id, provider_id, name, embedding_dims, source) "
            b"VALUES (%s, %s, 'e', 8, 'manual')",
            (model_id, provider_id),
        )
        conn.execute(
            b"INSERT INTO knowledge_bases (id, org_id, name, is_default, embedding_model_id, embedding_dims) "
            b"VALUES (%s, 'o', 'kb', true, %s, 8)",
            (kb_id, model_id),
        )
    assert migrate_up(url) == [4]
    with psycopg.connect(url) as conn:
        row = conn.execute(b"SELECT collection_name FROM knowledge_bases WHERE id = %s", (kb_id,)).fetchone()
    assert row == (f"kb_{model_id.hex}_8",)
    pipeline_tables = {"doc_nodes", "doc_blocks", "kb_tables", "embedding_cache"}
    assert pipeline_tables <= _tables(url)
    migrate_down(url, steps=1)
    assert not pipeline_tables & _tables(url)
