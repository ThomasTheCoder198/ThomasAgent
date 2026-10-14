from types import SimpleNamespace
from typing import Any

import pytest

from thomas_ragx.platform.config import Config
from thomas_ragx.retrieval import factory


def test_create_search_service_passes_the_metrics_recorder_to_embedder_and_reranker(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    seen: dict[str, Any] = {}

    class FakeEmbedder:
        def __init__(self, _retrying: object, *, metrics: object) -> None:
            seen["embedder"] = metrics

    class FakeReranker:
        def __init__(self, _retrying: object, *, metrics: object) -> None:
            seen["reranker"] = metrics

    monkeypatch.setattr(factory, "LiteLLMEmbedder", FakeEmbedder)
    monkeypatch.setattr(factory, "LiteLLMReranker", FakeReranker)
    recorder: Any = object()
    resources: Any = SimpleNamespace(pool=object(), qdrant=object())
    config = Config(minio_access_key="a", minio_secret_key="s")
    factory.create_search_service(config, resources, object(), object(), recorder)  # type: ignore[arg-type]
    assert seen["embedder"] is recorder and seen["reranker"] is recorder
