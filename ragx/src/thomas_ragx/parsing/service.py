import asyncio
import dataclasses
import time
from collections.abc import Collection, Mapping
from pathlib import Path
from typing import Final, Protocol

import structlog
from opentelemetry.trace import Span, Status, StatusCode, Tracer

from thomas_ragx.parsing.datalab import ConvertOutcome
from thomas_ragx.parsing.datalab_blocks import to_parsed_document
from thomas_ragx.parsing.locators import DATALAB_EXTENSIONS
from thomas_ragx.parsing.model import ParsedDocument, ParseProvider
from thomas_ragx.parsing.page_sizes import read_page_sizes
from thomas_ragx.parsing.registry import ParserRegistry
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.platform.tracing import ATTR_SPAN_KIND, SPAN_KIND_CHAIN

SPAN_DATALAB: Final = "rag.parse.datalab"
SPAN_LOCAL: Final = "rag.parse.local"
ATTR_PROVIDER: Final = "parse.provider"
ATTR_MODE: Final = "parse.mode"
ATTR_PAGES: Final = "parse.page_count"
ATTR_QUALITY: Final = "parse.quality"
ATTR_RUNTIME: Final = "parse.runtime_seconds"
ATTR_COST: Final = "parse.cost_cents"
COST_CENTS_KEY: Final = "final_cost_cents"  # Datalab's cost_breakdown field (spike S9)
log = structlog.get_logger(__name__)

type SpanAttributes = Mapping[str, str]


class Converter(Protocol):
    async def convert(self, path: Path, mode: str) -> ConvertOutcome: ...


def _needs_retry(outcome: ConvertOutcome, min_quality: float) -> bool:
    # None means "no score" (DOCX, PPTX, some PDFs), not "bad": a retry would pay again for nothing (S9-2).
    return outcome.quality is not None and outcome.quality < min_quality


def _better(first: ConvertOutcome, second: ConvertOutcome) -> ConvertOutcome:
    if second.quality is not None and first.quality is not None and second.quality > first.quality:
        return second
    return first


def _status_code(exc: Exception) -> int | None:
    """The HTTP status of a Datalab API error; never its message, which can echo request content."""
    code = getattr(exc, "status_code", None)
    return code if isinstance(code, int) else None


def _mark_failed(span: Span, exc: Exception) -> None:
    span.set_status(Status(StatusCode.ERROR, type(exc).__name__))


def _cost_cents(outcome: ConvertOutcome) -> float | None:
    value = (outcome.cost or {}).get(COST_CENTS_KEY)
    return float(value) if isinstance(value, int | float) and not isinstance(value, bool) else None


class ParseService:
    """Datalab-first for PDF and images, local-first for the extensions in `local_first_extensions`.

    Local otherwise and on every Datalab failure (main spec §4.2; owner decision O6). `degraded` means a
    quality loss: a local fallback for a type Datalab handles first. A local-first file is never degraded,
    whichever provider produced its blocks.

    When Datalab was enabled, its convert call raised, and the local parse then has no text, the outage is
    reported as the retryable `PROVIDER_UNAVAILABLE` instead of `INGEST_NO_TEXT` (ruling C3): a transient
    outage must not dead-letter every scan on its first delivery with a misleading reason.
    """

    def __init__(
        self,
        registry: ParserRegistry,
        converter: Converter | None,
        *,
        mode: str,
        retry_mode: str,
        min_quality: float,
        local_first_extensions: Collection[str],
        tracer: Tracer,
    ) -> None:
        self._registry = registry
        self._converter = converter
        self._mode = mode
        self._retry_mode = retry_mode
        self._min_quality = min_quality
        self._local_first = frozenset(extension.lower() for extension in local_first_extensions)
        self._tracer = tracer

    async def parse(
        self,
        path: Path,
        extension: str,
        *,
        external_parsing: bool,
        span_attributes: SpanAttributes | None = None,
    ) -> ParsedDocument:
        """`span_attributes` (request_id, org_id, kb_id, file_id) are set on every `rag.parse.*` span."""
        extension = extension.lower()
        attributes = dict(span_attributes or {})
        converter = self._converter if external_parsing else None
        if extension in self._local_first:
            parsed = await self._local_first_parse(path, extension, converter, attributes)
        else:
            parsed = await self._choose(path, extension, converter, attributes)
        if not parsed.blocks:
            raise AppError(ErrorCode.INGEST_NO_TEXT)
        return parsed

    async def _choose(
        self, path: Path, extension: str, converter: Converter | None, attributes: SpanAttributes
    ) -> ParsedDocument:
        datalab_type = extension in DATALAB_EXTENSIONS
        outage = False
        if datalab_type and converter is not None:
            outcome = await self._try_datalab(converter, path, attributes, "datalab failed, parsing locally")
            if outcome is not None:
                return await self._document(outcome, path, extension)
            outage = True
        try:
            parsed = await self._local(path, extension, attributes, degraded=datalab_type)
        except AppError as exc:
            if outage and exc.code is ErrorCode.INGEST_NO_TEXT:
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from None
            raise
        if outage and not parsed.blocks:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
        return parsed

    async def _local_first_parse(
        self, path: Path, extension: str, converter: Converter | None, attributes: SpanAttributes
    ) -> ParsedDocument:
        local: ParsedDocument | AppError
        try:
            local = await self._local(path, extension, attributes, degraded=False)
        except AppError as exc:
            local = exc
        if isinstance(local, ParsedDocument) and local.blocks:
            return local
        outage = False
        if converter is not None and extension in DATALAB_EXTENSIONS:
            outcome = await self._try_datalab(converter, path, attributes, "datalab rescue failed")
            if outcome is None:
                outage = True
            else:
                rescued = await self._document(outcome, path, extension)
                if rescued.blocks:
                    return rescued  # not degraded: a rescue improves on the local result (O6-1)
        if isinstance(local, AppError):
            if outage and local.code is ErrorCode.INGEST_NO_TEXT:
                raise AppError(ErrorCode.PROVIDER_UNAVAILABLE) from None
            raise local
        if outage:
            raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
        return local

    async def _try_datalab(
        self, converter: Converter, path: Path, attributes: SpanAttributes, failure_event: str
    ) -> ConvertOutcome | None:
        """Only the convert calls are guarded: a bug elsewhere must not pass for a Datalab outage."""
        try:
            return await self._fetch(converter, path, attributes)
        except Exception as exc:  # outage, rate limit, timeout: the local parser keeps ingest going
            log.warning(failure_event, error_type=type(exc).__name__, status_code=_status_code(exc))
            return None

    async def _fetch(self, converter: Converter, path: Path, attributes: SpanAttributes) -> ConvertOutcome:
        outcome = await self._convert(converter, path, self._mode, attributes)
        if _needs_retry(outcome, self._min_quality):
            try:
                retried = await self._convert(converter, path, self._retry_mode, attributes)
            except Exception as exc:  # keep the first result; it is still better than local
                log.warning(
                    "datalab retry failed", error_type=type(exc).__name__, status_code=_status_code(exc)
                )
            else:
                outcome = _better(outcome, retried)
        return outcome

    async def _convert(
        self, converter: Converter, path: Path, mode: str, attributes: SpanAttributes
    ) -> ConvertOutcome:
        started = time.perf_counter()
        # Exception recording and status-on-exception are off: a provider error message could carry request
        # content, and the SDK would copy it into the status description. The type name is all a span keeps.
        with self._tracer.start_as_current_span(
            SPAN_DATALAB,
            attributes={
                **attributes,
                ATTR_SPAN_KIND: SPAN_KIND_CHAIN,
                ATTR_PROVIDER: ParseProvider.DATALAB.value,
                ATTR_MODE: mode,
            },
            record_exception=False,
            set_status_on_exception=False,
        ) as span:
            try:
                outcome = await converter.convert(path, mode)
            except Exception as exc:
                _mark_failed(span, exc)
                raise
            if outcome.page_count is not None:
                span.set_attribute(ATTR_PAGES, outcome.page_count)
            if outcome.quality is not None:
                span.set_attribute(ATTR_QUALITY, outcome.quality)
            if outcome.runtime_seconds is not None:
                span.set_attribute(ATTR_RUNTIME, outcome.runtime_seconds)
            cost = _cost_cents(outcome)
            if cost is not None:
                span.set_attribute(ATTR_COST, cost)
        log.info("datalab convert completed", mode=mode, duration_ms=elapsed_ms(started))
        return outcome

    async def _document(self, outcome: ConvertOutcome, path: Path, extension: str) -> ParsedDocument:
        try:
            sizes = await asyncio.to_thread(read_page_sizes, path, extension)  # pixels until scaled (S9-3)
            return await asyncio.to_thread(
                to_parsed_document,
                outcome.tree,
                extension,
                page_count=outcome.page_count,
                quality=outcome.quality,
                page_sizes=sizes,
            )
        except Exception as exc:  # our mapping failed on a result Datalab delivered: not an outage
            log.warning("datalab result unreadable", error_type=type(exc).__name__)
            raise AppError(ErrorCode.INGEST_PARSE_FAILED) from None

    async def _local(
        self, path: Path, extension: str, attributes: SpanAttributes, *, degraded: bool
    ) -> ParsedDocument:
        parser = self._registry.for_extension(extension)
        if parser is None:
            raise AppError(ErrorCode.INGEST_NO_TEXT)  # an image without OCR (ruling A21)
        with self._tracer.start_as_current_span(
            SPAN_LOCAL,
            attributes={
                **attributes,
                ATTR_SPAN_KIND: SPAN_KIND_CHAIN,
                ATTR_PROVIDER: ParseProvider.LOCAL.value,
            },
            record_exception=False,
            set_status_on_exception=False,
        ) as span:
            try:
                parsed = await asyncio.to_thread(parser.parse, path)  # CPU-bound: keep the loop free
            except AppError as exc:
                _mark_failed(span, exc)
                raise
            except Exception as exc:
                _mark_failed(span, exc)
                raise AppError(ErrorCode.INGEST_PARSE_FAILED) from None
        return dataclasses.replace(parsed, degraded=degraded)
