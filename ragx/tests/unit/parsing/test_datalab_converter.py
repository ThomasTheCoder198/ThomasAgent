import asyncio
from pathlib import Path
from typing import Any, ClassVar

import pytest
from pydantic import SecretStr

from tests.unit.parsing.fixtures import datalab_tree
from thomas_ragx.parsing.datalab import DatalabConverter, create_converter
from thomas_ragx.platform.config import Config


class FakeResult:
    def __init__(self, success: bool = True) -> None:
        self.success = success
        self.json = datalab_tree() if success else None
        self.page_count = 2
        self.parse_quality_score = 4.5
        self.runtime = 1.5
        self.cost_breakdown = {"total_cost_cents": 1}
        self.error_in = None if success else "INFERENCE"


class FakeClient:
    created: ClassVar[list[dict[str, Any]]] = []
    calls: ClassVar[list[dict[str, Any]]] = []
    delay = 0.0
    success = True

    def __init__(self, **kwargs: Any) -> None:
        FakeClient.created.append(kwargs)

    async def __aenter__(self) -> "FakeClient":
        return self

    async def __aexit__(self, *_: object) -> None:
        return None

    async def convert(self, **kwargs: Any) -> FakeResult:
        FakeClient.calls.append(kwargs)
        await asyncio.sleep(FakeClient.delay)
        return FakeResult(FakeClient.success)


@pytest.fixture(autouse=True)
def reset() -> None:
    FakeClient.created, FakeClient.calls, FakeClient.delay, FakeClient.success = [], [], 0.0, True


def _converter(timeout: float = 5.0, **options: Any) -> DatalabConverter:
    return DatalabConverter(
        SecretStr("dl-key"),
        "https://ocr.example",
        timeout_seconds=timeout,
        poll_interval_seconds=1,
        client_factory=FakeClient,
        **options,
    )


async def test_converter_passes_key_url_mode_and_polling_explicitly(tmp_path: Path) -> None:
    outcome = await _converter().convert(tmp_path / "a.pdf", "balanced")
    assert FakeClient.created == [{"api_key": "dl-key", "base_url": "https://ocr.example", "timeout": 5}]
    options = FakeClient.calls[0]["options"]
    assert (options.mode, options.output_format, options.disable_image_captions) == (
        "balanced",
        "json",
        False,
    )
    assert (FakeClient.calls[0]["max_polls"], FakeClient.calls[0]["poll_interval"]) == (5, 1)
    assert (outcome.page_count, outcome.quality, outcome.runtime_seconds) == (2, 4.5, 1.5)


async def test_converter_leaves_the_sdk_cache_on_by_default(tmp_path: Path) -> None:
    await _converter().convert(tmp_path / "a.pdf", "balanced")
    # ruling A8-2: Datalab has a cache, follow the SDK
    assert FakeClient.calls[0]["options"].skip_cache is False


async def test_converter_passes_skip_cache_through_when_configured(tmp_path: Path) -> None:
    await _converter(skip_cache=True).convert(tmp_path / "a.pdf", "balanced")
    assert FakeClient.calls[0]["options"].skip_cache is True


async def test_converter_polls_at_least_once_when_the_timeout_is_shorter_than_the_interval(
    tmp_path: Path,
) -> None:
    await _converter(timeout=0.5).convert(tmp_path / "a.pdf", "balanced")
    assert FakeClient.calls[0]["max_polls"] == 1
    assert FakeClient.created[0]["timeout"] == 1


async def test_converter_gives_up_after_its_timeout(tmp_path: Path) -> None:
    FakeClient.delay = 1.0
    with pytest.raises(TimeoutError):
        await _converter(timeout=0.05).convert(tmp_path / "a.pdf", "balanced")


async def test_converter_raises_when_datalab_reports_failure(tmp_path: Path) -> None:
    FakeClient.success = False
    with pytest.raises(RuntimeError):
        await _converter().convert(tmp_path / "a.pdf", "balanced")


def _config(**overrides: Any) -> Config:
    return Config(minio_access_key="a", minio_secret_key="s", **overrides)


@pytest.mark.parametrize("key", [None, "", "   "])
def test_create_converter_returns_none_without_a_usable_key(key: str | None) -> None:
    assert create_converter(_config(datalab_api_key=key)) is None


async def test_create_converter_passes_the_config_through(tmp_path: Path) -> None:
    config = _config(
        datalab_api_key="dl-key",
        ocr_base_url="https://ocr.example",
        datalab_timeout_seconds=30.0,
        datalab_poll_interval_seconds=3,
        datalab_skip_cache=True,
    )
    converter = create_converter(config, client_factory=FakeClient)
    assert isinstance(converter, DatalabConverter)
    await converter.convert(tmp_path / "a.pdf", "balanced")
    assert FakeClient.created == [{"api_key": "dl-key", "base_url": "https://ocr.example", "timeout": 30}]
    call = FakeClient.calls[0]
    assert (call["max_polls"], call["poll_interval"], call["options"].skip_cache) == (10, 3, True)
