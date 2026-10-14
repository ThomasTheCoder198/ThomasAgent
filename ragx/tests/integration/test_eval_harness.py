from pathlib import Path
from typing import Any

import pytest
from minio import Minio
from qdrant_client import AsyncQdrantClient

from tests.integration.search_support import (
    assign_fake_rerank_model,
    indexed_corpus,
    registry_for,
    search_service_for,
)
from thomas_ragx.db.pool import Pool
from thomas_ragx.eval.golden import load_golden
from thomas_ragx.eval.runner import EvalRunner
from thomas_ragx.kb import repository as kbs

pytestmark = pytest.mark.integration
ORG = "org-eval"
GOLDEN = Path(__file__).resolve().parents[1] / "eval" / "synthetic.jsonl"
RECALL_AT_5_FLOOR = 0.7  # synthetic set with fake models; record the real value in the task report


async def test_eval_harness_reports_metrics_on_the_synthetic_set(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    async with pool.connection() as conn:
        kb = await kbs.get_kb(conn, ORG, corpus.kb_id)
    assert kb is not None
    report = await EvalRunner(pool, search_service_for(pool, qdrant, tmp_path, registry)).run(
        ORG, kb, load_golden(GOLDEN)
    )
    data = report.to_json()
    assert (data["items"], data["scored"], data["unresolved"]) == (34, 30, [])
    assert data["recall"]["5"] >= RECALL_AT_5_FLOOR and data["recall"]["8"] >= data["recall"]["5"]
    assert 0.0 < data["mrr"] <= 1.0
    assert data["locatorAccuracy"] == 1.0  # payload locators come from the same parse as doc_blocks
    assert data["noEvidenceRate"] is not None and data["latencyMs"]["p95"] >= data["latencyMs"]["p50"]
    assert data["ingest"]["files"] == 5 and data["ingest"]["filesPerMinute"] > 0


async def test_eval_ignores_files_search_cannot_return(
    pool: Pool, minio_config: dict[str, Any], minio: Minio, qdrant: AsyncQdrantClient, tmp_path: Path
) -> None:
    corpus = await indexed_corpus(pool, minio_config, minio, qdrant, tmp_path, ORG)
    registry = registry_for(pool)
    await assign_fake_rerank_model(registry)
    broken = "chinh-sach-doi-tra.md"
    async with pool.connection() as conn:
        await conn.execute(b"UPDATE kb_files SET status = 'failed' WHERE id = %s", (corpus.file_ids[broken],))
        kb = await kbs.get_kb(conn, ORG, corpus.kb_id)
    assert kb is not None
    items = load_golden(GOLDEN)
    report = await EvalRunner(pool, search_service_for(pool, qdrant, tmp_path, registry)).run(ORG, kb, items)
    expected_unresolved = sorted({i.id for i in items if any(e.file == broken for e in i.expected)})
    assert expected_unresolved
    assert report.unresolved == expected_unresolved
