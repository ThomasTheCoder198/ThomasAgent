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
    assert {"orgs", "providers", "models", "model_roles", "knowledge_bases", "kb_files", "outbox"} <= _tables(
        url
    )
    assert migrate_up(url) == []
    reverted = migrate_down(url, steps=len(applied))
    assert reverted == list(reversed(applied))
    assert _tables(url) == {"schema_migrations"}
    assert migrate_up(url) == applied
