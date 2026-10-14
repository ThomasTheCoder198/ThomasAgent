import uuid
from datetime import UTC, datetime
from pathlib import Path

import pytest
from qdrant_client import AsyncQdrantClient

from tests.rag_fakes import HashEmbedder
from thomas_ragx.ingest.chunker import Chunker
from thomas_ragx.ingest.outline import build_outline
from thomas_ragx.parsing.model import BlockCollector, BlockType, text_lines
from thomas_ragx.retrieval.qdrant_store import PointOwner, QdrantStore, build_point, tenant_filter
from thomas_ragx.retrieval.segmenter import IdentitySegmenter
from thomas_ragx.retrieval.sparse import SparseEncoder

pytestmark = pytest.mark.integration
DIMS = 16
COLLECTION = f"kb_{uuid.uuid4().hex}_{DIMS}"


async def _points(owner: PointOwner, texts: list[str], tmp_path: Path) -> list:
    collector = BlockCollector()
    for n, text in enumerate(texts, start=1):
        collector.add(BlockType.TEXT, text, text_lines(n, n))
    blocks = collector.blocks()
    chunks = Chunker(3, 700, 1).chunk(
        owner.file_id,
        owner.version,
        "a.md",
        blocks,
        build_outline(owner.file_id, owner.version, "a.md", blocks),
    )
    dense = await HashEmbedder(DIMS).embed(None, [c.embed_text for c in chunks])  # type: ignore[arg-type]
    sparse = SparseEncoder(IdentitySegmenter(), tmp_path, k=1.2, b=0.75, avg_len=256.0).encode_passages(
        [c.text for c in chunks]
    )
    return [
        build_point(owner, c, d, s, "a.md", "md", datetime.now(UTC))
        for c, d, s in zip(chunks, dense, sparse, strict=True)
    ]


def test_tenant_filter_refuses_missing_ids() -> None:
    with pytest.raises(ValueError):
        tenant_filter("", uuid.uuid4())
    with pytest.raises(ValueError):
        tenant_filter("org", "")


async def test_ensure_collection_is_idempotent(qdrant: AsyncQdrantClient) -> None:
    store = QdrantStore(qdrant, upsert_batch_size=2)
    await store.ensure_collection(COLLECTION, DIMS)
    await store.ensure_collection(COLLECTION, DIMS)
    info = await qdrant.get_collection(COLLECTION)
    assert set(info.payload_schema) >= {"org_id", "kb_id", "file_id"}


async def test_replace_file_points_leaves_no_orphans(qdrant: AsyncQdrantClient, tmp_path: Path) -> None:
    store = QdrantStore(qdrant, upsert_batch_size=2)
    await store.ensure_collection(COLLECTION, DIMS)
    owner = PointOwner("org-a", uuid.uuid4(), uuid.uuid4(), 1)
    flt = tenant_filter(owner.org_id, owner.kb_id)
    await store.replace_file_points(
        COLLECTION, owner, await _points(owner, ["một hai ba", "bốn năm sáu", "bảy tám chín"], tmp_path)
    )
    assert await store.count(COLLECTION, flt) == 3
    await store.replace_file_points(COLLECTION, owner, await _points(owner, ["chỉ còn một chunk"], tmp_path))
    assert await store.count(COLLECTION, flt) == 1


async def test_points_of_one_org_are_invisible_to_another(qdrant: AsyncQdrantClient, tmp_path: Path) -> None:
    store = QdrantStore(qdrant, upsert_batch_size=8)
    await store.ensure_collection(COLLECTION, DIMS)
    kb_id = uuid.uuid4()  # identical KB id and content: only org_id tells them apart
    owner_a, owner_b = (
        PointOwner("org-a", kb_id, uuid.uuid4(), 1),
        PointOwner("org-b", kb_id, uuid.uuid4(), 1),
    )
    for owner in (owner_a, owner_b):
        await store.replace_file_points(
            COLLECTION, owner, await _points(owner, ["cùng một nội dung"], tmp_path)
        )
    found, _ = await qdrant.scroll(COLLECTION, scroll_filter=tenant_filter("org-a", kb_id), limit=10)
    assert {p.payload["org_id"] for p in found} == {"org-a"}
    await store.delete_kb_points(COLLECTION, "org-a", kb_id)
    assert await store.count(COLLECTION, tenant_filter("org-a", kb_id)) == 0
    assert await store.count(COLLECTION, tenant_filter("org-b", kb_id)) == 1


async def test_payload_carries_tenant_blocks_and_locators(qdrant: AsyncQdrantClient, tmp_path: Path) -> None:
    store = QdrantStore(qdrant, upsert_batch_size=8)
    await store.ensure_collection(COLLECTION, DIMS)
    owner = PointOwner("org-a", uuid.uuid4(), uuid.uuid4(), 2)
    points = await _points(owner, ["Hợp đồng có hiệu lực"], tmp_path)
    await store.replace_file_points(COLLECTION, owner, points)
    (point,), _ = await qdrant.scroll(COLLECTION, scroll_filter=tenant_filter("org-a", owner.kb_id), limit=1)
    payload = point.payload or {}
    assert (payload["org_id"], payload["kb_id"], payload["file_id"], payload["version"]) == (
        "org-a",
        str(owner.kb_id),
        str(owner.file_id),
        2,
    )
    assert payload["blocks"][0]["locator"] == {"kind": "text_lines", "lineStart": 1, "lineEnd": 1}
    assert payload["lang"] == "vi" and payload["text"] == "Hợp đồng có hiệu lực"
