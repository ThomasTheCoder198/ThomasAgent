from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from minio import Minio
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from qdrant_client import AsyncQdrantClient

from tests.integration.search_support import (
    assign_fake_rerank_model,
    indexed_corpus,
    registry_for,
    search_service_for,
)
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import repository as kbs
from thomas_ragx.kb.model import KnowledgeBase
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.model import Capability, Role
from thomas_ragx.retrieval.search_service import SearchFilters

pytestmark = pytest.mark.integration
ORG = "org-search"


async def _kb(pool: Pool, kb_id: UUID) -> KnowledgeBase:
    async with pool.connection() as conn:
        kb = await kbs.get_kb(conn, ORG, kb_id)
        assert kb is not None
        return kb


async def test_search_labels_blocks_with_their_text_and_locator(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    service = search_service_for(pool, qdrant, tmp_path, registry)
    answer = await service.search(
        ORG, await _kb(pool, corpus.kb_id), "Được đổi trả hàng trong bao lâu?", SearchFilters()
    )
    assert not answer.insufficient_evidence and 1 <= len(answer.results) <= 8
    first = answer.results[0]
    assert first.source == 1 and first.blocks[0].label == "[1.1]"
    assert any("7 ngày" in block.text for result in answer.results for block in result.blocks)
    async with pool.connection() as conn:
        for source, result in enumerate(answer.results, start=1):
            assert result.source == source and result.blocks
            for index, block in enumerate(result.blocks, start=1):
                assert block.label == f"[{source}.{index}]"
                stored = await (
                    await conn.execute(
                        b"SELECT text, locator FROM doc_blocks WHERE file_id = %s AND block_id = %s",
                        (result.file_id, block.block_id),
                    )
                ).fetchone()
                assert stored is not None
                assert (stored["text"], stored["locator"]) == (block.text, block.locator)


async def test_search_requires_a_rerank_model(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    service = search_service_for(pool, qdrant, tmp_path, registry_for(pool))
    with pytest.raises(AppError) as caught:
        await service.search(ORG, await _kb(pool, corpus.kb_id), "bảo hành", SearchFilters())
    assert caught.value.code is ErrorCode.LLM_ROLE_NOT_ASSIGNED


async def test_a_decision_only_rerank_model_is_refused_until_m6(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    seeded = (await registry.roles())[Role.EMBEDDING]
    assert seeded is not None
    jev = await registry.add_manual_model(
        seeded.provider_id, "jev", frozenset({Capability.DECISION}), None, None
    )
    await registry.assign_role(Role.RERANK, jev.id)
    with pytest.raises(AppError) as caught:
        await search_service_for(pool, qdrant, tmp_path, registry).search(
            ORG, await _kb(pool, corpus.kb_id), "bảo hành", SearchFilters()
        )
    assert caught.value.code is ErrorCode.LLM_MODEL_CAPABILITY_MISMATCH


async def test_search_drops_points_of_deleted_files(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    returns = corpus.file_ids["chinh-sach-doi-tra.md"]
    async with pool.connection() as conn:  # deleted, purge not run yet: its points still exist
        await conn.execute(b"UPDATE kb_files SET deleted_at = now() WHERE id = %s", (returns,))
    answer = await search_service_for(pool, qdrant, tmp_path, registry).search(
        ORG, await _kb(pool, corpus.kb_id), "đổi trả hàng 7 ngày", SearchFilters()
    )
    assert returns not in {r.file_id for r in answer.results}


async def test_filters_narrow_to_files(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    only = corpus.file_ids["quy-che-nhan-su.md"]
    answer = await search_service_for(pool, qdrant, tmp_path, registry).search(
        ORG, await _kb(pool, corpus.kb_id), "ngày", SearchFilters(file_ids=(only,))
    )
    assert answer.results and not answer.insufficient_evidence
    assert {r.file_id for r in answer.results} == {only}


async def test_unrelated_query_means_insufficient_evidence(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    answer = await search_service_for(pool, qdrant, tmp_path, registry).search(
        ORG, await _kb(pool, corpus.kb_id), "zxqv wplk", SearchFilters()
    )
    assert answer.results == [] and answer.insufficient_evidence


async def test_search_spans_hide_the_query_by_default(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    service = search_service_for(pool, qdrant, tmp_path, registry, tracer=provider.get_tracer(__name__))
    await service.search(ORG, await _kb(pool, corpus.kb_id), "lõi lọc bảo hành", SearchFilters())
    spans = exporter.get_finished_spans()
    assert {s.name for s in spans} >= {"rag.retrieval.search", "rag.retrieval.rerank"}
    for span in spans:
        for payload in (str(span.attributes), str(span.events), str(span.status.description)):
            assert "lõi lọc" not in payload


async def test_search_error_spans_hide_content_in_events_and_status(
    pool: Pool,
    minio_config: dict[str, Any],
    minio: Minio,
    qdrant: AsyncQdrantClient,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from opentelemetry.trace import StatusCode

    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    service = search_service_for(pool, qdrant, tmp_path, registry, tracer=provider.get_tracer(__name__))
    marker = "synthetic-private-query-document-key"

    async def failed(*args: Any, **kwargs: Any) -> Any:
        raise RuntimeError(marker)

    monkeypatch.setattr(service._reranker, "rerank", failed)
    try:
        with pytest.raises(RuntimeError):
            await service.search(ORG, await _kb(pool, corpus.kb_id), "ngay " + marker, SearchFilters())
        spans = exporter.get_finished_spans()
        assert {span.name for span in spans} >= {"rag.retrieval.search", "rag.retrieval.rerank"}
        for span in spans:
            assert span.status.status_code is StatusCode.ERROR
            for payload in (str(span.attributes), str(span.events), str(span.status.description)):
                assert marker not in payload
    finally:
        provider.shutdown()
