from uuid import uuid4

from qdrant_client.models import ScoredPoint

from thomas_ragx.retrieval.qdrant_store import FIELD_VERSION
from thomas_ragx.retrieval.search_service import SearchAnswer, _result
from thomas_ragx.retrieval.tool_dto import search_output

STORED_VERSION = 3


def _point() -> ScoredPoint:
    return ScoredPoint(
        id=str(uuid4()),
        version=0,
        score=0.9,
        payload={
            "chunk_id": "c-1",
            "file_id": str(uuid4()),
            "file_name": "policy.pdf",
            FIELD_VERSION: STORED_VERSION,
            "node_id": str(uuid4()),
            "section": ["Hoàn tiền"],
            "page_start": 1,
            "page_end": 1,
            "text": "Hoàn tiền trong 7 ngày.",
            "blocks": [
                {"block_id": "b-1", "char_start": 0, "char_end": 9, "locator": {"kind": "text_lines"}}
            ],
        },
    )


def test_search_results_name_the_file_version_their_blocks_belong_to() -> None:
    result = _result(1, _point(), 0.9)
    assert result.file_version == STORED_VERSION
    wire = search_output(SearchAnswer(results=[result], insufficient_evidence=False)).model_dump(
        by_alias=True
    )
    assert wire["results"][0]["fileVersion"] == STORED_VERSION
