from collections.abc import AsyncIterator, Iterator
from typing import Any

import pytest
from testcontainers.minio import MinioContainer
from testcontainers.postgres import PostgresContainer
from testcontainers.redis import RedisContainer

from thomas_ragx.db.migrate import migrate_up
from thomas_ragx.db.pool import Pool, create_pool

POSTGRES_IMAGE = "postgres:18.6-alpine"
REDIS_IMAGE = "redis:8.10.2"
MINIO_IMAGE = "pgsty/minio:RELEASE.2026-08-04T00-00-00Z"
TRUNCATE_ALL = (
    b"TRUNCATE outbox, idempotency_keys, kb_files, knowledge_bases, "
    b"orgs, model_roles, models, providers CASCADE"
)


@pytest.fixture(scope="session")
def postgres_url() -> Iterator[str]:
    with PostgresContainer(POSTGRES_IMAGE, driver=None) as container:
        yield container.get_connection_url()


@pytest.fixture(scope="session")
def migrated_url(postgres_url: str) -> str:
    migrate_up(postgres_url)
    return postgres_url


@pytest.fixture
async def pool(migrated_url: str) -> AsyncIterator[Pool]:
    created = create_pool(migrated_url, min_size=1, max_size=4)
    await created.open(wait=True)
    async with created.connection() as conn:
        await conn.execute(TRUNCATE_ALL)
    yield created
    await created.close()


@pytest.fixture(scope="session")
def redis_url() -> Iterator[str]:
    with RedisContainer(REDIS_IMAGE) as container:
        host = container.get_container_host_ip()
        port = container.get_exposed_port(container.port)
        yield f"redis://{host}:{port}/0"


@pytest.fixture(scope="session")
def minio_config() -> Iterator[dict[str, Any]]:
    with MinioContainer(MINIO_IMAGE) as container:
        yield container.get_config()
