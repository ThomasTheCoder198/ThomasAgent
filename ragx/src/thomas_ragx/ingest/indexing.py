from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from pathlib import Path
from uuid import UUID

from thomas_ragx.ingest.chunker import Chunker
from thomas_ragx.ingest.embedding_cache import CachedEmbedder
from thomas_ragx.ingest.summaries import NodeSummarizer
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.retry import retrying
from thomas_ragx.providers.chat import LiteLLMChat
from thomas_ragx.providers.embedding import LiteLLMEmbedder
from thomas_ragx.registry.model import ModelHandle, Role
from thomas_ragx.registry.service import LLMProviderService
from thomas_ragx.resources import Resources
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.segmenter import create_segmenter
from thomas_ragx.retrieval.sparse import SparseEncoder
from thomas_ragx.worker.metrics import WorkerMetrics

BM25_DIR_NAME = "bm25"


@dataclass(frozen=True)
class Indexing:
    chunker: Chunker
    embedder: CachedEmbedder
    sparse: SparseEncoder
    store: QdrantStore
    summarizer: NodeSummarizer
    models: Callable[[UUID], Awaitable[ModelHandle]]


def create_indexing(
    config: Config, resources: Resources, registry: LLMProviderService, metrics: WorkerMetrics
) -> Indexing:
    return Indexing(
        chunker=Chunker(config.chunk_target_words, config.chunk_max_words, config.chunk_overlap_words),
        embedder=CachedEmbedder(
            resources.pool,
            LiteLLMEmbedder(lambda: retrying(config), metrics=metrics),
            batch_size=config.embedding_batch_size,
            metrics=metrics,
        ),
        sparse=SparseEncoder(
            create_segmenter(config.vi_tokenizer),
            Path(config.work_dir) / BM25_DIR_NAME,
            k=config.bm25_k,
            b=config.bm25_b,
            avg_len=config.bm25_avg_len,
        ),
        store=QdrantStore(resources.qdrant, upsert_batch_size=config.qdrant_upsert_batch_size),
        summarizer=NodeSummarizer(
            LiteLLMChat(lambda: retrying(config), metrics=metrics),
            lambda: registry.role_handle(Role.CHAT_FAST),
            enabled=config.node_summaries_enabled,
            max_nodes=config.node_summary_max_nodes,
            input_chars=config.node_summary_input_chars,
            max_tokens=config.node_summary_max_tokens,
        ),
        models=registry.model_handle,
    )
