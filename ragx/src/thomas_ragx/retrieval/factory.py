from pathlib import Path
from typing import Final

from opentelemetry.trace import Tracer

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.retry import retrying
from thomas_ragx.platform.timing import MILLISECONDS_PER_SECOND
from thomas_ragx.providers.embedding import LiteLLMEmbedder
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.resources import Resources
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.rerank import LiteLLMReranker
from thomas_ragx.retrieval.search_service import SearchService
from thomas_ragx.retrieval.segmenter import create_segmenter
from thomas_ragx.retrieval.sparse import SparseEncoder
from thomas_ragx.worker.metrics import WorkerMetrics

# Ingest builds the same encoder; both sides must agree on the directory so queries use the ingest vocabulary.
BM25_DIR_NAME: Final = "bm25"


def create_search_service(
    config: Config,
    resources: Resources,
    registry: LLMProviderService,
    tracer: Tracer,
    metrics: WorkerMetrics,
) -> SearchService:
    """Shared by the API and the eval CLI; the recorder keeps provider calls on the RED metrics (rule 6)."""
    return SearchService(
        resources.pool,
        registry,
        LiteLLMEmbedder(lambda: retrying(config), metrics=metrics),
        SparseEncoder(
            create_segmenter(config.vi_tokenizer),
            Path(config.work_dir) / BM25_DIR_NAME,
            k=config.bm25_k,
            b=config.bm25_b,
            avg_len=config.bm25_avg_len,
        ),
        QdrantStore(resources.qdrant, upsert_batch_size=config.qdrant_upsert_batch_size),
        LiteLLMReranker(
            lambda: retrying(config),
            metrics=metrics,
            timeout_seconds=config.tool_timeout_ms / MILLISECONDS_PER_SECOND,
        ),
        tracer,
        dense_limit=config.search_dense_limit,
        sparse_limit=config.search_sparse_limit,
        fused_limit=config.search_fused_limit,
        result_limit=config.search_result_limit,
        min_score=config.search_min_rerank_score,
        trace_content=config.trace_content,
    )
