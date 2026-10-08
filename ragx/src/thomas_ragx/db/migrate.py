import re
from dataclasses import dataclass
from importlib.resources import files
from typing import Final

import psycopg
import structlog
from psycopg.rows import TupleRow

MIGRATION_FILE: Final = re.compile(r"^(\d{4})_([a-z0-9_]+)\.(up|down)\.sql$")
# Any fixed 64-bit value works; every ragx migrator must use the same one so two bootstraps never interleave.
ADVISORY_LOCK_KEY: Final = 7161005
DIRECTION_UP: Final = "up"
DIRECTION_DOWN: Final = "down"
CREATE_BOOKKEEPING: Final = b"""
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    integer     PRIMARY KEY,
    name       text        NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
)"""
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class Migration:
    version: int
    name: str
    up: bytes
    down: bytes


def load_migrations() -> list[Migration]:
    found: dict[int, dict[str, object]] = {}
    for entry in files("thomas_ragx.db").joinpath("migrations").iterdir():
        match = MIGRATION_FILE.match(entry.name)
        if match is None:
            continue
        version, name, direction = int(match.group(1)), match.group(2), match.group(3)
        found.setdefault(version, {"name": name})[direction] = entry.read_bytes()
    migrations = []
    for version in sorted(found):
        parts = found[version]
        up, down = parts.get(DIRECTION_UP), parts.get(DIRECTION_DOWN)
        if not isinstance(up, bytes) or not isinstance(down, bytes):
            raise TypeError(f"migration {version:04d} needs both .up.sql and .down.sql")
        migrations.append(Migration(version=version, name=str(parts["name"]), up=up, down=down))
    return migrations


def _applied(conn: psycopg.Connection[TupleRow]) -> list[int]:
    rows = conn.execute(b"SELECT version FROM schema_migrations ORDER BY version").fetchall()
    return [int(row[0]) for row in rows]


def _prepare(conn: psycopg.Connection[TupleRow]) -> None:
    conn.execute(b"SELECT pg_advisory_lock(%s)", (ADVISORY_LOCK_KEY,))
    conn.execute(CREATE_BOOKKEEPING)


def migrate_up(conninfo: str) -> list[int]:
    """Apply pending migrations, each in its own transaction; returns the versions applied."""
    done: list[int] = []
    with psycopg.connect(conninfo, autocommit=True) as conn:
        _prepare(conn)
        applied = set(_applied(conn))
        for migration in load_migrations():
            if migration.version in applied:
                continue
            with conn.transaction():
                conn.execute(migration.up)
                conn.execute(
                    b"INSERT INTO schema_migrations (version, name) VALUES (%s, %s)",
                    (migration.version, migration.name),
                )
            log.info("migration applied", version=migration.version, migration=migration.name)
            done.append(migration.version)
    return done


def migrate_down(conninfo: str, steps: int) -> list[int]:
    """Revert the newest `steps` applied migrations; returns the versions reverted, newest first."""
    by_version = {migration.version: migration for migration in load_migrations()}
    done: list[int] = []
    with psycopg.connect(conninfo, autocommit=True) as conn:
        _prepare(conn)
        for version in list(reversed(_applied(conn)))[:steps]:
            migration = by_version[version]
            with conn.transaction():
                conn.execute(migration.down)
                conn.execute(b"DELETE FROM schema_migrations WHERE version = %s", (version,))
            log.info("migration reverted", version=version, migration=migration.name)
            done.append(version)
    return done
