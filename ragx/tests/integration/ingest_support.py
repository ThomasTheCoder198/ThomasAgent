from typing import Any

from minio import Minio
from opentelemetry.sdk.trace import TracerProvider

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
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.segmenter import IdentitySegmenter
from thomas_ragx.retrieval.sparse import SparseEncoder


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
