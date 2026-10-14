from typing import Any
from uuid import uuid4

import litellm
import pytest
from opentelemetry.metrics import NoOpMeterProvider
from pydantic import SecretStr

from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.retry import retrying
from thomas_ragx.providers.embedding import LiteLLMEmbedder
from thomas_ragx.registry.model import (
    Capability,
    ModelHandle,
    ModelRecord,
    ModelSource,
    ProviderAccess,
    ProviderKind,
)
from thomas_ragx.worker.metrics import WorkerMetrics


def _handle() -> ModelHandle:
    record = ModelRecord(
        uuid4(),
        uuid4(),
        "text-embedding-3-small",
        frozenset({Capability.EMBEDDING}),
        False,
        4,
        None,
        None,
        None,
        ModelSource.MANUAL,
    )
    return ModelHandle(record, ProviderAccess(ProviderKind.OPENAI, "", SecretStr("sk-test")))


async def test_embedder_passes_the_key_per_call_and_keeps_order(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: dict[str, Any] = {}

    async def fake(**kwargs: Any) -> Any:
        seen.update(kwargs)
        return type(
            "R", (), {"data": [{"embedding": [0.1] * 4, "index": 1}, {"embedding": [0.2] * 4, "index": 0}]}
        )

    monkeypatch.setattr(litellm, "aembedding", fake)
    config = Config(minio_access_key="a", minio_secret_key="s")
    vectors = await LiteLLMEmbedder(
        lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
    ).embed(_handle(), ["x", "y"])
    assert seen["model"] == "openai/text-embedding-3-small" and seen["api_key"] == "sk-test"
    assert vectors == [[0.2] * 4, [0.1] * 4]  # sorted by the response index


async def test_embedder_retries_provider_outages_then_gives_up(monkeypatch: pytest.MonkeyPatch) -> None:
    calls = 0

    async def down(**_: Any) -> Any:
        nonlocal calls
        calls += 1
        raise litellm.APIConnectionError("down", "openai", "m")

    monkeypatch.setattr(litellm, "aembedding", down)
    config = Config(
        minio_access_key="a", minio_secret_key="s", retry_base_delay_seconds=0.0, retry_max_delay_seconds=0.0
    )
    with pytest.raises(AppError) as caught:
        await LiteLLMEmbedder(
            lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
        ).embed(_handle(), ["x"])
    assert caught.value.code is ErrorCode.PROVIDER_UNAVAILABLE and calls == config.retry_max_attempts


async def test_embedder_honors_retry_after(monkeypatch: pytest.MonkeyPatch) -> None:
    import httpx

    delays: list[float] = []
    calls = 0

    async def down(**kwargs: Any) -> Any:
        nonlocal calls
        calls += 1
        response = httpx.Response(
            429, headers={"Retry-After": "2"}, request=httpx.Request("POST", "https://p.example")
        )
        raise litellm.RateLimitError("private provider error", "openai", "m", response=response)

    config = Config(
        minio_access_key="a",
        minio_secret_key="s",
        retry_max_attempts=2,
        retry_base_delay_seconds=0.0,
        retry_max_delay_seconds=0.0,
    )

    async def sleep(delay: float) -> None:
        delays.append(delay)

    def factory() -> Any:
        result = retrying(config)
        result.sleep = sleep
        return result

    monkeypatch.setattr(litellm, "aembedding", down)
    with pytest.raises(AppError):
        await LiteLLMEmbedder(factory, metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))).embed(
            _handle(), ["private content"]
        )
    assert calls == 2
    assert delays == [2.0]


@pytest.mark.parametrize("indices", [[0, 0], [0, 2], [-1, 0]])
async def test_embedder_rejects_invalid_response_indices(
    monkeypatch: pytest.MonkeyPatch, indices: list[int]
) -> None:
    async def fake(**kwargs: Any) -> Any:
        return type("R", (), {"data": [{"embedding": [0.1] * 4, "index": index} for index in indices]})

    monkeypatch.setattr(litellm, "aembedding", fake)
    config = Config(minio_access_key="a", minio_secret_key="s")
    with pytest.raises(AppError) as caught:
        await LiteLLMEmbedder(
            lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
        ).embed(_handle(), ["first", "second"])
    assert caught.value.code is ErrorCode.INGEST_EMBEDDING_MISMATCH


async def test_embedding_metrics_use_the_injected_exported_recorder(monkeypatch: pytest.MonkeyPatch) -> None:
    from opentelemetry.sdk.metrics import MeterProvider
    from opentelemetry.sdk.metrics.export import InMemoryMetricReader

    from thomas_ragx.worker.metrics import WorkerMetrics

    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    recorder = WorkerMetrics(provider.get_meter(__name__))
    config = Config(minio_access_key="a", minio_secret_key="s", retry_max_attempts=1)
    calls = 0

    async def fake(**kwargs: Any) -> Any:
        nonlocal calls
        calls += 1
        if calls > 1:
            raise litellm.AuthenticationError("rejected", "openai", "m")
        return type("R", (), {"data": [{"embedding": [0.1] * 4, "index": 0}]})

    monkeypatch.setattr(litellm, "aembedding", fake)
    try:
        embedder = LiteLLMEmbedder(lambda: retrying(config), metrics=recorder)
        await embedder.embed(_handle(), ["first"])
        with pytest.raises(AppError):
            await embedder.embed(_handle(), ["second"])
        data = reader.get_metrics_data()
        metrics = {
            metric.name: metric
            for resource in data.resource_metrics
            for scope in resource.scope_metrics
            for metric in scope.metrics
        }
        for name in ("ragx_embedding_calls_total", "ragx_embedding_call_duration_seconds"):
            points = metrics[name].data.data_points
            assert {point.attributes["outcome"] for point in points} == {"success", "error"}
            assert all(
                dict(point.attributes)
                in ({"kind": "openai", "outcome": "success"}, {"kind": "openai", "outcome": "error"})
                for point in points
            )
        assert sum(point.value for point in metrics["ragx_embedding_calls_total"].data.data_points) == 2
        assert (
            sum(point.count for point in metrics["ragx_embedding_call_duration_seconds"].data.data_points)
            == 2
        )
    finally:
        provider.shutdown()
