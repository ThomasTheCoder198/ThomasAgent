import base64
import json
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

import httpx
from fastapi import FastAPI
from minio import Minio
from qdrant_client import AsyncQdrantClient
from redis.asyncio import Redis

from tests.tokens import mint, public_keys_config
from thomas_ragx.api.app import create_app
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.platform.config import Config
from thomas_ragx.resources import Resources

UNREACHABLE_REDIS = "redis://127.0.0.1:1/0"
UNREACHABLE_QDRANT = "http://127.0.0.1:1"
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
    pool: Pool,
    redis_url: str | None = None,
    minio_config: dict[str, Any] | None = None,
    qdrant_url: str | None = None,
    **overrides: Any,
) -> AsyncIterator[tuple[httpx.AsyncClient, FastAPI]]:
    storage_settings: dict[str, Any] = {}
    if minio_config is not None:
        storage_settings = {
            "minio_access_key": minio_config["access_key"],
            "minio_secret_key": minio_config["secret_key"],
            "s3_public_endpoint": f"http://{minio_config['endpoint']}",
        }
    config = Config(
        **{"minio_access_key": "a", "minio_secret_key": "s", **storage_settings},
        otlp_endpoint="",
        core_public_keys=public_keys_config(),
        **overrides,
    )
    resources = Resources(
        pool=pool,
        redis=Redis.from_url(redis_url or UNREACHABLE_REDIS, decode_responses=True, protocol=2),
        storage=_storage(minio_config),
        qdrant=AsyncQdrantClient(url=qdrant_url or UNREACHABLE_QDRANT),
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
        await resources.qdrant.close()


async def presign(client: httpx.AsyncClient, org_id: str, kb_id: str, name: str, size: int) -> httpx.Response:
    return await client.post(
        f"/v1/knowledge-bases/{kb_id}/uploads",
        json={"name": name, "sizeBytes": size},
        headers=bearer(org_id, "kb:write"),
    )


async def post_to_storage(ticket: dict[str, Any], content: bytes, name: str) -> httpx.Response:
    """What the browser does: a multipart POST straight to MinIO, policy fields first, the file last."""
    async with httpx.AsyncClient() as storage:
        return await storage.post(
            ticket["upload"]["url"], data=ticket["upload"]["fields"], files={"file": (name, content)}
        )


async def confirm(
    client: httpx.AsyncClient, org_id: str, kb_id: str, file_id: str, request_id: str | None = None
) -> httpx.Response:
    headers = bearer(org_id, "kb:write")
    if request_id is not None:
        headers = {
            "Authorization": f"Bearer {mint(org_id, 'kb:write', rid=request_id)}",
            "X-Request-Id": request_id,
        }
    return await client.post(f"/v1/knowledge-bases/{kb_id}/files/{file_id}/confirm", headers=headers)


async def upload(
    client: httpx.AsyncClient,
    org_id: str,
    kb_id: str,
    name: str,
    content: bytes,
    request_id: str | None = None,
) -> httpx.Response:
    presigned = await presign(client, org_id, kb_id, name, len(content))
    assert presigned.status_code == 201, presigned.text
    ticket = presigned.json()["data"]
    stored = await post_to_storage(ticket, content, name)
    assert stored.status_code == 204, stored.text
    return await confirm(client, org_id, kb_id, ticket["fileId"], request_id)
