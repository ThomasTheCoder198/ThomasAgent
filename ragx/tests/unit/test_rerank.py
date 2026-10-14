from typing import Any

import httpx
import litellm
import pytest
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader
from structlog.contextvars import bound_contextvars
from structlog.testing import capture_logs

from tests.unit.test_embedder import _handle
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.retry import retrying
from thomas_ragx.retrieval.rerank import LiteLLMReranker, RerankScore
from thomas_ragx.worker.metrics import WorkerMetrics


async def test_reranker_passes_per_call_credentials_and_maps_scores(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: dict[str, Any] = {}

    async def fake(**kwargs: Any) -> Any:
        seen.update(kwargs)
        return type(
            "R", (), {"results": [{"index": 1, "relevance_score": 0.9}, {"index": 0, "relevance_score": 0.2}]}
        )

    monkeypatch.setattr(litellm, "arerank", fake)
    config = Config(minio_access_key="a", minio_secret_key="s")
    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    try:
        reranker = LiteLLMReranker(
            lambda: retrying(config), metrics=WorkerMetrics(provider.get_meter(__name__))
        )
        with bound_contextvars(request_id="req_rerank"), capture_logs() as logs:
            scores = await reranker.rerank(_handle(), "private query", ["private first", "private second"], 2)
        assert scores == [RerankScore(1, 0.9), RerankScore(0, 0.2)]
        assert seen == {
            "model": "openai/text-embedding-3-small",
            "query": "private query",
            "documents": ["private first", "private second"],
            "top_n": 2,
            "api_key": "sk-test",
            "api_base": None,
        }
        assert logs[0]["request_id"] == "req_rerank" and logs[0]["duration_ms"] >= 0
        assert all(secret not in str(logs) for secret in ("private", "sk-test"))
        data = reader.get_metrics_data()
        assert data is not None
        metrics = {
            metric.name: metric
            for resource in data.resource_metrics
            for scope in resource.scope_metrics
            for metric in scope.metrics
        }
        for name in ("ragx_rerank_calls_total", "ragx_rerank_call_duration_seconds"):
            assert [dict(p.attributes) for p in metrics[name].data.data_points] == [
                {"kind": "openai", "outcome": "success"}
            ]
    finally:
        provider.shutdown()


@pytest.mark.parametrize("retryable", [True, False])
async def test_reranker_errors_metrics_safe_logs_and_retry_after(
    monkeypatch: pytest.MonkeyPatch, retryable: bool
) -> None:
    delays: list[float] = []
    calls = 0

    async def failed(**kwargs: Any) -> Any:
        nonlocal calls
        calls += 1
        if retryable:
            response = httpx.Response(
                429, headers={"Retry-After": "2"}, request=httpx.Request("POST", "https://p.example")
            )
            raise litellm.RateLimitError("private sk-test", "openai", "m", response=response)
        raise litellm.AuthenticationError("private sk-test", "openai", "m")

    monkeypatch.setattr(litellm, "arerank", failed)
    config = Config(
        minio_access_key="a",
        minio_secret_key="s",
        retry_max_attempts=2,
        retry_base_delay_seconds=0,
        retry_max_delay_seconds=0,
    )

    async def sleep(delay: float) -> None:
        delays.append(delay)

    def factory() -> Any:
        result = retrying(config)
        result.sleep = sleep
        return result

    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    try:
        reranker = LiteLLMReranker(factory, metrics=WorkerMetrics(provider.get_meter(__name__)))
        with (
            bound_contextvars(request_id="req_rerank"),
            capture_logs() as logs,
            pytest.raises(AppError) as caught,
        ):
            await reranker.rerank(_handle(), "private", ["private"], 1)
        assert caught.value.code is (
            ErrorCode.PROVIDER_UNAVAILABLE if retryable else ErrorCode.LLM_PROVIDER_REJECTED
        )
        assert calls == (2 if retryable else 1)
        assert delays == ([2.0] if retryable else [])
        assert all(secret not in str(logs) for secret in ("private", "sk-test"))
        assert all(log["request_id"] == "req_rerank" and log["duration_ms"] >= 0 for log in logs)
        data = reader.get_metrics_data()
        assert data is not None
        metrics = {
            metric.name: metric
            for resource in data.resource_metrics
            for scope in resource.scope_metrics
            for metric in scope.metrics
        }
        points = metrics["ragx_rerank_calls_total"].data.data_points
        assert sum(p.value for p in points) == calls
        assert [dict(p.attributes) for p in points] == [{"kind": "openai", "outcome": "error"}]
    finally:
        provider.shutdown()


@pytest.mark.parametrize(
    "results",
    [
        [{"index": -1, "relevance_score": 0.9}],
        [{"index": 2, "relevance_score": 0.9}],
        [{"index": 0.5, "relevance_score": 0.9}],
        [{"index": "1", "relevance_score": 0.9}],
        [{"index": True, "relevance_score": 0.9}],
        [{"index": 0, "relevance_score": 0.9}, {"index": 0, "relevance_score": 0.8}],
        [{"index": 0, "relevance_score": float("inf")}],
        [{"index": 0, "relevance_score": float("-inf")}],
        [{"index": 0, "relevance_score": float("nan")}],
        [{"index": 0, "relevance_score": "private malformed score"}],
        [{"index": 0, "relevance_score": True}],
        [{"index": 0}],
        [None],
        {"private malformed result": 1},
    ],
)
async def test_reranker_rejects_malformed_results_without_retry_or_content(
    monkeypatch: pytest.MonkeyPatch,
    results: Any,
) -> None:
    calls = 0

    async def fake(**kwargs: Any) -> Any:
        nonlocal calls
        calls += 1
        return type("R", (), {"results": results})

    monkeypatch.setattr(litellm, "arerank", fake)
    config = Config(
        minio_access_key="a", minio_secret_key="s", retry_base_delay_seconds=0, retry_max_delay_seconds=0
    )
    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    try:
        reranker = LiteLLMReranker(
            lambda: retrying(config), metrics=WorkerMetrics(provider.get_meter(__name__))
        )
        with capture_logs() as logs, pytest.raises(AppError) as caught:
            await reranker.rerank(_handle(), "private query", ["private first", "private second"], 2)
        assert caught.value.code is ErrorCode.LLM_PROVIDER_REJECTED
        assert caught.value.__cause__ is None
        assert calls == 1
        assert "private" not in str(logs) + str(caught.value) + repr(caught.value)
        data = reader.get_metrics_data()
        assert data is not None
        metrics = {
            metric.name: metric
            for resource in data.resource_metrics
            for scope in resource.scope_metrics
            for metric in scope.metrics
        }
        assert [dict(p.attributes) for p in metrics["ragx_rerank_calls_total"].data.data_points] == [
            {"kind": "openai", "outcome": "error"}
        ]
    finally:
        provider.shutdown()
