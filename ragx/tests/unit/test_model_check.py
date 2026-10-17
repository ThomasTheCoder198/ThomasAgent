from typing import Any
from uuid import uuid4

import pytest
import structlog
from structlog.testing import capture_logs

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry import model_check
from thomas_ragx.registry.model import Capability, ModelRecord, ModelSource, ProviderAccess, ProviderKind
from thomas_ragx.registry.model_check import LiteLLMModelChecker

ACCESS = ProviderAccess(ProviderKind.OPENROUTER, "", None)
MODEL = ModelRecord(
    uuid4(),
    uuid4(),
    "acme/embed",
    frozenset({Capability.EMBEDDING}),
    False,
    None,
    None,
    None,
    None,
    ModelSource.MANUAL,
)


class Response:
    def __init__(self, vector: list[float]) -> None:
        self.data = [{"embedding": vector}]


async def test_a_failed_probe_logs_one_event_and_maps_to_provider_rejected(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    import litellm

    monkeypatch.setattr(model_check, "log", structlog.get_logger("test"))

    async def boom(**_: Any) -> Any:
        raise RuntimeError("401 sk-secret")

    monkeypatch.setattr(litellm, "aembedding", boom)
    with capture_logs() as logs, pytest.raises(AppError) as failure:
        await LiteLLMModelChecker(1.0).measure_embedding_dims(ACCESS, MODEL)
    assert failure.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    assert len(logs) == 1
    event = logs[0]
    assert (event["outcome"], event["error_type"]) == ("error", "RuntimeError")
    assert "duration_ms" in event
    assert "sk-secret" not in str(logs)


async def test_a_successful_probe_logs_one_event_and_returns_the_vector_length(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    import litellm

    monkeypatch.setattr(model_check, "log", structlog.get_logger("test"))

    async def fine(**_: Any) -> Any:
        return Response([0.1, 0.2, 0.3])

    monkeypatch.setattr(litellm, "aembedding", fine)
    with capture_logs() as logs:
        dims = await LiteLLMModelChecker(1.0).measure_embedding_dims(ACCESS, MODEL)
    assert dims == 3
    assert [(event["outcome"], event["log_level"]) for event in logs] == [("success", "info")]
