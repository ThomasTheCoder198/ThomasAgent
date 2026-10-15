import asyncio
import unicodedata
from collections.abc import Awaitable
from typing import Any, Final, TypeVar
from uuid import UUID

from opentelemetry.trace import Status, StatusCode, Tracer
from psycopg.rows import DictRow

from thomas_ragx.auth.caller import Caller
from thomas_ragx.db.pool import Pool
from thomas_ragx.kb import repository as kbs
from thomas_ragx.kb.file_service import CODE_INVALID, CODE_OUT_OF_RANGE, decode_cursor, encode_cursor_values
from thomas_ragx.kb.model import KnowledgeBase
from thomas_ragx.kb.repository import Connection
from thomas_ragx.kb.service import require_org
from thomas_ragx.kb.upload import field_error
from thomas_ragx.platform.constants import HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.tracing import ATTR_SPAN_KIND, ATTR_TOOL, SPAN_KIND_TOOL
from thomas_ragx.retrieval import tool_repository as tools
from thomas_ragx.retrieval.search_service import SearchAnswer, SearchFilters, SearchService

T = TypeVar("T")
SUFFIX_SEPARATOR: Final = "."


class KbToolService:
    """Use only signed org and KB claims to scope tool data (ruling C2)."""

    def __init__(
        self,
        pool: Pool,
        search: SearchService,
        tracer: Tracer,
        *,
        timeout_seconds: float,
        section_max_chars: int,
        section_page_rows: int,
        outline_max_nodes: int,
        page_default_limit: int,
        page_max_limit: int,
    ) -> None:
        self._pool = pool
        self._search = search
        self._tracer = tracer
        self._timeout = timeout_seconds
        self._section_max_chars = section_max_chars
        self._section_page_rows = section_page_rows
        self._outline_max_nodes = outline_max_nodes
        self._page_default = page_default_limit
        self._page_max = page_max_limit

    async def _bounded(self, tool: str, work: Awaitable[T]) -> T:
        with self._tracer.start_as_current_span(
            f"rag.tool.{tool}",
            attributes={ATTR_SPAN_KIND: SPAN_KIND_TOOL, ATTR_TOOL: tool},
            record_exception=False,
            set_status_on_exception=False,
        ) as span:
            try:
                async with asyncio.timeout(self._timeout):  # ruling C7
                    return await work
            except TimeoutError:
                span.set_status(Status(StatusCode.ERROR))
                raise AppError(ErrorCode.DEPENDENCY_TIMEOUT) from None
            except AppError as error:
                # A 4xx is the caller's mistake (unknown file or node), not a failed tool.
                if error.http_status() >= HTTP_STATUS_SERVER_ERROR:
                    span.set_status(Status(StatusCode.ERROR))
                raise
            except Exception:
                span.set_status(Status(StatusCode.ERROR))
                raise

    async def _kb(self, caller: Caller) -> KnowledgeBase:
        if caller.kb_id is None:
            raise AppError(ErrorCode.FORBIDDEN)
        try:
            kb_id = UUID(caller.kb_id)
        except ValueError:
            raise AppError(ErrorCode.KB_NOT_FOUND) from None
        async with self._pool.connection() as conn:
            await require_org(conn, caller.org_id)
            kb = await kbs.get_kb(conn, caller.org_id, kb_id)
        if kb is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)  # never FORBIDDEN: do not reveal another org's KB
        return kb

    async def search(self, caller: Caller, tool: str, query: str, filters: SearchFilters) -> SearchAnswer:
        async def run() -> SearchAnswer:
            return await self._search.search(caller.org_id, await self._kb(caller), query, filters)

        return await self._bounded(tool, run())

    async def outline(self, caller: Caller, tool: str, file_id: UUID) -> tuple[str, list[DictRow], bool]:
        async def run() -> tuple[str, list[DictRow], bool]:
            kb = await self._kb(caller)
            async with self._pool.connection() as conn:
                file = await tools.searchable_file(conn, caller.org_id, kb.id, file_id)
                if file is None:
                    raise AppError(ErrorCode.KB_FILE_NOT_FOUND)
                nodes = await tools.outline_nodes(
                    conn, caller.org_id, kb.id, file_id, self._outline_max_nodes + 1
                )
            return file["name"], nodes[: self._outline_max_nodes], len(nodes) > self._outline_max_nodes

        return await self._bounded(tool, run())

    async def read_section(self, caller: Caller, tool: str, node_id: UUID) -> dict[str, Any]:
        async def run() -> dict[str, Any]:
            kb = await self._kb(caller)
            async with self._pool.connection() as conn:
                node = await tools.node_in_kb(conn, caller.org_id, kb.id, node_id)
                if node is None:
                    raise AppError(ErrorCode.KB_NODE_NOT_FOUND)
                path = await tools.node_path(conn, caller.org_id, kb.id, node_id)
                kept, truncated = await self._capped_section(
                    conn, caller.org_id, kb.id, node["file_id"], node_id
                )
            return {"node": node, "path": path, "blocks": kept, "truncated": truncated}

        return await self._bounded(tool, run())

    async def _capped_section(
        self, conn: Connection, org_id: str, kb_id: UUID, file_id: UUID, node_id: UUID
    ) -> tuple[list[DictRow], bool]:
        # Paged so a huge section is never read whole: at most one page past the cap is fetched (Codex I2).
        kept: list[DictRow] = []
        used = 0
        after = tools.BEFORE_FIRST_POSITION
        while True:
            page = await tools.section_blocks(
                conn, org_id, kb_id, file_id, node_id, after_position=after, limit=self._section_page_rows
            )
            for row in page:
                remaining = self._section_max_chars - used
                if len(row["text"]) > remaining:
                    # Preserve whole blocks when possible, while bounding an oversized first block.
                    if not kept:
                        kept.append({**row, "text": row["text"][:remaining]})
                    return kept, True
                kept.append(row)
                used += len(row["text"])
            if len(page) < self._section_page_rows:
                return kept, False
            after = page[-1]["position"]

    async def list_documents(
        self,
        caller: Caller,
        tool: str,
        name_contains: str | None,
        doc_type: str | None,
        cursor: str | None,
        limit: int | None,
    ) -> tuple[list[DictRow], str | None, int]:
        size = self._page_default if limit is None else limit
        if size < 1 or size > self._page_max:
            raise field_error("limit", CODE_OUT_OF_RANGE)
        if doc_type is not None and not doc_type.isalnum():
            raise field_error("docType", CODE_INVALID)

        async def run() -> tuple[list[DictRow], str | None, int]:
            kb = await self._kb(caller)
            after = decode_cursor(cursor) if cursor else None
            # Stored names are NFC (kb.upload.normalize_file_name), so an NFD query must match them.
            stored_name = unicodedata.normalize("NFC", name_contains) if name_contains is not None else None
            suffix = SUFFIX_SEPARATOR + doc_type.lower() if doc_type else None
            async with self._pool.connection() as conn:
                rows = await tools.list_documents(
                    conn, caller.org_id, kb.id, stored_name, suffix, after, size + 1
                )
            page = rows[:size]
            return (
                page,
                (encode_cursor_values(page[-1]["created_at"], page[-1]["id"]) if len(rows) > size else None),
                size,
            )

        return await self._bounded(tool, run())
