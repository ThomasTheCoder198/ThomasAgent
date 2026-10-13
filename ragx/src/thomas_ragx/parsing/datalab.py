import asyncio
import math
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Final

from datalab_sdk import AsyncDatalabClient, ConvertOptions
from pydantic import SecretStr

from thomas_ragx.platform.config import Config

OUTPUT_FORMAT: Final = "json"
MIN_POLLS: Final = 1  # a submitted (and billed) job must be polled at least once


@dataclass(frozen=True)
class ConvertOutcome:
    tree: dict[str, Any]
    page_count: int | None
    # None means Datalab gave no score (DOCX, PPTX, some PDFs); it is not 0.0 (ruling S9-2).
    quality: float | None
    runtime_seconds: float | None
    cost: dict[str, Any] | None


class DatalabFailedError(RuntimeError):
    """Datalab answered but reported a failed conversion; the message is its error stage, never content."""


class DatalabConverter:
    """Key and address are always explicit: the SDK would otherwise read DATALAB_* at import (ruling A5)."""

    def __init__(
        self,
        api_key: SecretStr,
        base_url: str,
        *,
        timeout_seconds: float,
        poll_interval_seconds: int,
        skip_cache: bool = False,
        client_factory: Callable[..., Any] = AsyncDatalabClient,
    ) -> None:
        # False is the SDK default: Datalab's result cache stays on (ruling A8-2).
        self._skip_cache = skip_cache
        self._api_key = api_key
        self._base_url = base_url
        self._timeout = timeout_seconds
        self._poll = poll_interval_seconds
        self._client_factory = client_factory

    async def convert(self, path: Path, mode: str) -> ConvertOutcome:
        # Captions stay on: they are the figure text of M1.3 (ruling A21).
        options = ConvertOptions(
            mode=mode, output_format=OUTPUT_FORMAT, disable_image_captions=False, skip_cache=self._skip_cache
        )
        async with asyncio.timeout(self._timeout):  # the SDK's own retries can wait minutes (ruling A6)
            async with self._client_factory(
                api_key=self._api_key.get_secret_value(),
                base_url=self._base_url,
                timeout=max(MIN_POLLS, math.ceil(self._timeout)),
            ) as client:
                result = await client.convert(
                    file_path=path,
                    options=options,
                    max_polls=max(MIN_POLLS, int(self._timeout // self._poll)),
                    poll_interval=self._poll,
                )
        if not result.success or result.json is None:
            raise DatalabFailedError(str(result.error_in))
        return ConvertOutcome(
            tree=result.json,
            page_count=result.page_count,
            quality=result.parse_quality_score,
            runtime_seconds=result.runtime,
            cost=result.cost_breakdown,
        )


def create_converter(
    config: Config, *, client_factory: Callable[..., Any] = AsyncDatalabClient
) -> DatalabConverter | None:
    """A missing or blank key means no converter: ingest then parses locally and marks files degraded."""
    if config.datalab_api_key is None or not config.datalab_api_key.get_secret_value().strip():
        return None
    return DatalabConverter(
        config.datalab_api_key,
        config.ocr_base_url,
        timeout_seconds=config.datalab_timeout_seconds,
        poll_interval_seconds=config.datalab_poll_interval_seconds,
        skip_cache=config.datalab_skip_cache,
        client_factory=client_factory,
    )
