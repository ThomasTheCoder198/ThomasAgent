import base64
import json
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

import httpx
from fastapi import FastAPI
from minio import Minio
from redis.asyncio import Redis

from tests.tokens import mint, public_keys_config
from thomas_ragx.api.app import create_app
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.platform.config import Config
from thomas_ragx.resources import Resources

UNREACHABLE_REDIS = "redis://127.0.0.1:1/0"
UNREACHABLE_MINIO = "127.0.0.1:1"


def bearer(org_id: str, scope: str, kb_id: str | None = None) -> dict[str, str]:
    token = mint(org_id, scope, kb_id)
    claims = json.loads(base64.urlsafe_b64decode(token.split(".")[1] + "=="))
    return {"Authorization": f"Bearer {token}", "X-Request-Id": claims["rid"]}


def _storage(minio_config: dict[str, Any] | None) -> ObjectStorage:
    if minio_config is None:
        return ObjectStorage(Minio(UNREACHABLE_MINIO, access_key="a", secret_key="b", secure=False))
    return ObjectStorage(
        Minio(
            minio_config["endpoint"],
            access_key=minio_config["access_key"],
            secret_key=minio_config["secret_key"],
            secure=False,
        )
    )


@asynccontextmanager
async def api_client(
    pool: Pool, redis_url: str | None = None, minio_config: dict[str, Any] | None = None, **overrides: Any
) -> AsyncIterator[tuple[httpx.AsyncClient, FastAPI]]:
    config = Config(
        minio_access_key="a",
        minio_secret_key="s",
        otlp_endpoint="",
        core_public_keys=public_keys_config(),
        **overrides,
    )
    resources = Resources(
        pool=pool,
        redis=Redis.from_url(redis_url or UNREACHABLE_REDIS, decode_responses=True, protocol=2),
        storage=_storage(minio_config),
        connect_timeout_seconds=1.0,
        readiness_timeout_seconds=1.0,
    )
    app = create_app(config, resources=resources)
    try:
        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app), base_url="http://ragx"
        ) as client:
            yield client, app
    finally:
        await resources.redis.aclose()
