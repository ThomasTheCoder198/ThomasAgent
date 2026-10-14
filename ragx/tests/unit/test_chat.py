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
from thomas_ragx.providers.chat import LiteLLMChat
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
        "small-chat",
        frozenset({Capability.CHAT}),
        False,
        4,
        None,
        None,
        None,
        ModelSource.MANUAL,
    )
    return ModelHandle(record, ProviderAccess(ProviderKind.OPENAI, "", SecretStr("sk-test")))


async def test_chat_passes_per_call_access_prompt_and_token_limit(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: dict[str, Any] = {}

    async def fake(**kwargs: Any) -> Any:
        seen.update(kwargs)
        return type(
            "R",
            (),
            {
                "choices": [
                    type("Choice", (), {"message": type("Message", (), {"content": "  summary  "})()})()
                ]
            },
        )

    monkeypatch.setattr(litellm, "acompletion", fake)
    config = Config(minio_access_key="a", minio_secret_key="s")
    summary = await LiteLLMChat(
        lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
    ).complete(_handle(), "section text", 120)
    assert seen["model"] == "openai/small-chat" and seen["api_key"] == "sk-test"
    assert summary == "summary"
    assert seen["messages"] == [{"role": "user", "content": "section text"}]
    assert seen["max_tokens"] == 120
    assert "request_id" not in seen and "metadata" not in seen


async def test_chat_retries_provider_outages_then_gives_up(monkeypatch: pytest.MonkeyPatch) -> None:
    calls = 0

    async def down(**_: Any) -> Any:
        nonlocal calls
        calls += 1
        raise litellm.APIConnectionError("down", "openai", "m")

    monkeypatch.setattr(litellm, "acompletion", down)
    config = Config(
        minio_access_key="a", minio_secret_key="s", retry_base_delay_seconds=0.0, retry_max_delay_seconds=0.0
    )
    with pytest.raises(AppError) as caught:
        await LiteLLMChat(
            lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
        ).complete(_handle(), "x", 120)
    assert caught.value.code is ErrorCode.PROVIDER_UNAVAILABLE and calls == config.retry_max_attempts


async def test_chat_honors_retry_after(monkeypatch: pytest.MonkeyPatch) -> None:
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

    monkeypatch.setattr(litellm, "acompletion", down)
    with pytest.raises(AppError):
        await LiteLLMChat(factory, metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))).complete(
            _handle(), "private content", 120
        )
    assert calls == 2
    assert delays == [2.0]


async def test_chat_metrics_use_the_injected_exported_recorder(monkeypatch: pytest.MonkeyPatch) -> None:
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
        return type(
            "R",
            (),
            {"choices": [type("Choice", (), {"message": type("Message", (), {"content": "summary"})()})()]},
        )

    monkeypatch.setattr(litellm, "acompletion", fake)
    try:
        embedder = LiteLLMChat(lambda: retrying(config), metrics=recorder)
        await embedder.complete(_handle(), "first", 120)
        with pytest.raises(AppError):
            await embedder.complete(_handle(), "second", 120)
        data = reader.get_metrics_data()
        metrics = {
            metric.name: metric
            for resource in data.resource_metrics
            for scope in resource.scope_metrics
            for metric in scope.metrics
        }
        for name in ("ragx_chat_calls_total", "ragx_chat_call_duration_seconds"):
            points = metrics[name].data.data_points
            assert {point.attributes["outcome"] for point in points} == {"success", "error"}
            assert all(
                dict(point.attributes)
                in ({"kind": "openai", "outcome": "success"}, {"kind": "openai", "outcome": "error"})
                for point in points
            )
        assert sum(point.value for point in metrics["ragx_chat_calls_total"].data.data_points) == 2
        assert sum(point.count for point in metrics["ragx_chat_call_duration_seconds"].data.data_points) == 2
    finally:
        provider.shutdown()


async def test_chat_completion_logs_are_safe_on_success_and_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    from structlog.testing import capture_logs

    async def success(**kwargs: Any) -> Any:
        return type("R", (), {"choices": [type("C", (), {"message": type("M", (), {"content": None})()})()]})

    async def failure(**kwargs: Any) -> Any:
        raise litellm.AuthenticationError("private secret provider detail", "openai", "m")

    config = Config(minio_access_key="a", minio_secret_key="s", retry_max_attempts=1)
    chat = LiteLLMChat(
        lambda: retrying(config), metrics=WorkerMetrics(NoOpMeterProvider().get_meter(__name__))
    )
    with capture_logs() as logs:
        monkeypatch.setattr(litellm, "acompletion", success)
        assert await chat.complete(_handle(), "private section", 120) == ""
        monkeypatch.setattr(litellm, "acompletion", failure)
        with pytest.raises(AppError) as caught:
            await chat.complete(_handle(), "private section", 120)
    assert caught.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    calls = [entry for entry in logs if entry["event"] == "chat call completed"]
    assert [entry["outcome"] for entry in calls] == ["success", "error"]
    assert all(entry["kind"] == "openai" and entry["duration_ms"] >= 0 for entry in calls)
    assert "private" not in str(logs) and "sk-test" not in str(logs)
