from typing import Any

from minio import Minio
from opentelemetry.sdk.trace import TracerProvider

from tests.integration.api_client import api_client, bearer, upload
from tests.rag_fakes import FakeChat, HashEmbedder
from thomas_ragx.db.pool import Pool
from thomas_ragx.ingest.chunker import Chunker
from thomas_ragx.ingest.embedding_cache import CachedEmbedder
from thomas_ragx.ingest.indexing import Indexing
from thomas_ragx.ingest.pipeline import IngestPipeline
from thomas_ragx.ingest.summaries import NodeSummarizer
from thomas_ragx.kb.storage import ObjectStorage
from thomas_ragx.parsing.registry import local_registry
from thomas_ragx.parsing.service import ParseService
from thomas_ragx.platform.config import Config
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.segmenter import IdentitySegmenter
from thomas_ragx.retrieval.sparse import SparseEncoder

ORG = "org-ingest"
REQUEST_ID = "req_5b1f0c2e-1d4a-4b7e-8f21-0c6d5e4a3b99"
MARKDOWN = "# Chính sách\n\nĐoạn mở đầu.\n\n## Đổi trả\n\nTrong 7 ngày.\n".encode()


async def uploaded_file(
    pool: Pool, minio_config: dict[str, Any], name: str, content: bytes
) -> tuple[str, str]:
    """Provisions `ORG`, uploads one file through the real API and returns `(kb_id, file_id)`."""
    async with api_client(pool, minio_config=minio_config) as (client, app):
        await seed_dev_registry(app.state.registry_service)
        org = await client.put(
            f"/v1/orgs/{ORG}",
            json={"name": "Ingest", "externalParsing": True},
            headers=bearer(ORG, "org:provision"),
        )
        kb_id = org.json()["data"]["defaultKbId"]
        stored = await upload(client, ORG, kb_id, name, content, REQUEST_ID)
    assert stored.status_code == 200, stored.text
    return kb_id, stored.json()["data"]["id"]


async def file_row(pool: Pool, file_id: str) -> dict[str, Any]:
    async with pool.connection() as conn:
        row = await (await conn.execute(b"SELECT * FROM kb_files WHERE id = %s", (file_id,))).fetchone()
    assert row is not None
    return dict(row)


def pipeline_for(
    pool: Pool,
    minio: Minio,
    qdrant: Any,
    tmp_path: Any,
    *,
    embedder: Any = None,
    chat: Any = None,
    models: Any = None,
) -> IngestPipeline:
    tracer = TracerProvider().get_tracer(__name__)
    parser = ParseService(
        local_registry(),
        None,
        mode="balanced",
        retry_mode="accurate",
        min_quality=3.0,
        tracer=tracer,
        local_first_extensions=frozenset({".docx", ".pptx"}),
    )
    registry = create_registry_service(Config(minio_access_key="a", minio_secret_key="s"), pool)

    async def models_for_chat():
        return object()  # FakeChat does not inspect the model.

    indexing = Indexing(
        Chunker(350, 700, 50),
        CachedEmbedder(pool, embedder or HashEmbedder(1536), batch_size=100),
        SparseEncoder(IdentitySegmenter(), tmp_path / "bm25", k=1.2, b=0.75, avg_len=256.0),
        QdrantStore(qdrant, upsert_batch_size=64),
        NodeSummarizer(
            chat or FakeChat(),
            models_for_chat,
            enabled=chat is not None,
            max_nodes=50,
            input_chars=1000,
            max_tokens=50,
        ),
        models or registry.model_handle,
    )
    return IngestPipeline(
        pool,
        ObjectStorage(minio),
        parser,
        tracer,
        indexing=indexing,
        work_dir=tmp_path / "work",
        lease_seconds=60.0,
    )
