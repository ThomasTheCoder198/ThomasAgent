import socket
from pathlib import Path

import pytest

from thomas_ragx.retrieval.segmenter import UndertheseaSegmenter
from thomas_ragx.retrieval.sparse import SparseEncoder


def _encoder(tmp_path: Path) -> SparseEncoder:
    return SparseEncoder(UndertheseaSegmenter(), tmp_path / "bm25", k=1.2, b=0.75, avg_len=256.0)


def test_sparse_encoder_builds_without_network(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    def refuse(*_: object, **__: object) -> None:
        raise AssertionError("network access")

    monkeypatch.setattr(socket.socket, "connect", refuse)
    vector = _encoder(tmp_path).encode_passages(["Hợp đồng lao động có hiệu lực"])[0]
    assert vector.indices and len(vector.indices) == len(vector.values)


def test_query_and_passage_share_compound_tokens(tmp_path: Path) -> None:
    encoder = _encoder(tmp_path)
    passage = encoder.encode_passages(["Thời hạn hợp đồng lao động là 12 tháng"])[0]
    query = encoder.encode_query("hợp đồng lao động")
    assert set(query.indices) <= set(passage.indices)
    assert all(isinstance(v, float) for v in query.values)  # FastEmbed returns int32 ones for queries
