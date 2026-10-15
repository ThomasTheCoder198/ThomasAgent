from dataclasses import dataclass
from pathlib import Path
from typing import Any
from uuid import UUID

import httpx
from minio import Minio
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.trace import Tracer
from qdrant_client import AsyncQdrantClient

from tests.integration.api_client import api_client, bearer, upload
from tests.integration.ingest_support import pipeline_for
from tests.rag_fakes import FakeReranker, HashEmbedder
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.job import IngestJob
from thomas_ragx.platform.config import Config
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.registry.model import Capability, ModelRecord, Role
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.search_service import SearchService
from thomas_ragx.retrieval.segmenter import IdentitySegmenter
from thomas_ragx.retrieval.sparse import SparseEncoder
from thomas_ragx.retrieval.tool_service import KbToolService


async def call_tool(
    client: httpx.AsyncClient, tool: str, body: dict[str, Any], kb_id: UUID | str | None, org: str
) -> httpx.Response:
    """POST a tool route with a token for `org` whose `kb` claim is `kb_id` (None: no claim)."""
    claim = None if kb_id is None else str(kb_id)
    return await client.post(f"/v1/tools/{tool}", json=body, headers=bearer(org, f"tool:{tool}", claim))


@dataclass(frozen=True)
class IndexedCorpus:
    kb_id: UUID
    file_ids: dict[str, UUID]


def registry_for(pool: Pool) -> LLMProviderService:
    return create_registry_service(Config(minio_access_key="a", minio_secret_key="s"), pool)


async def assign_fake_rerank_model(service: LLMProviderService) -> ModelRecord:
    seeded = (await service.roles())[Role.EMBEDDING]
    assert seeded is not None
    model = await service.add_manual_model(
        seeded.provider_id, "fake-rerank", frozenset({Capability.RERANK}), None, None
    )
    await service.assign_role(Role.RERANK, model.id)
    return model


def search_service_for(
    pool: Pool,
    qdrant: AsyncQdrantClient,
    tmp_path: Path,
    registry: LLMProviderService,
    *,
    trace_content: bool = False,
    tracer: Tracer | None = None,
) -> SearchService:
    config = Config(minio_access_key="a", minio_secret_key="s")
    return SearchService(
        pool,
        registry,
        HashEmbedder(1536),
        SparseEncoder(
            IdentitySegmenter(),
            tmp_path / "bm25",
            k=config.bm25_k,
            b=config.bm25_b,
            avg_len=config.bm25_avg_len,
        ),
        QdrantStore(qdrant, upsert_batch_size=64),
        FakeReranker(),
        tracer or TracerProvider().get_tracer(__name__),
        dense_limit=config.search_dense_limit,
        sparse_limit=config.search_sparse_limit,
        fused_limit=config.search_fused_limit,
        result_limit=config.search_result_limit,
        min_score=config.search_min_rerank_score,
        trace_content=trace_content,
    )


def tool_service_for(
    pool: Pool, qdrant: AsyncQdrantClient, tmp_path: Path, registry: LLMProviderService
) -> KbToolService:
    return KbToolService(
        pool,
        search_service_for(pool, qdrant, tmp_path, registry),
        TracerProvider().get_tracer(__name__),
        timeout_seconds=20,
        section_max_chars=8000,
        outline_max_nodes=500,
        page_default_limit=20,
        page_max_limit=100,
    )


async def indexed_corpus(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Path,
    org_id: str,
) -> IndexedCorpus:
    files: dict[str, UUID] = {}
    async with api_client(pool, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        response = await client.put(
            f"/v1/orgs/{org_id}",
            json={"name": "Search", "externalParsing": False},
            headers=bearer(org_id, "org:provision"),
        )
        assert response.status_code == 200, response.text
        kb_id = UUID(response.json()["data"]["defaultKbId"])
        pipeline = pipeline_for(pool, minio, qdrant, tmp_path)
        for path in sorted((Path(__file__).parents[1] / "eval" / "corpus").iterdir()):
            response = await upload(
                client,
                org_id,
                str(kb_id),
                path.name,
                path.read_bytes(),
                "req_5b1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b99",
            )
            assert response.status_code == 200, response.text
            file_id = UUID(response.json()["data"]["id"])
            files[path.name] = file_id
            await pipeline.run(
                IngestJob(org_id, kb_id, file_id, 1, "req_5b1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b99")
            )
    return IndexedCorpus(kb_id, files)
