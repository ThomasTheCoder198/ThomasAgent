from typing import Any

from fastapi import Request

from thomas_ragx.api.routing import api_router
from thomas_ragx.api.services import ToolServiceDep
from thomas_ragx.auth.caller import (
    TOOL_KB_LIST_DOCUMENTS,
    TOOL_KB_OUTLINE,
    TOOL_KB_READ_SECTION,
    TOOL_KB_SEARCH,
)
from thomas_ragx.auth.dependencies import (
    KbListDocumentsCaller,
    KbOutlineCaller,
    KbReadSectionCaller,
    KbSearchCaller,
)
from thomas_ragx.kb.dto import to_json
from thomas_ragx.platform.http_response import page_body, request_id_of, success_body
from thomas_ragx.retrieval.search_service import SearchFilters
from thomas_ragx.retrieval.tool_dto import (
    KbListDocumentsInput,
    KbOutlineInput,
    KbReadSectionInput,
    KbSearchInput,
    document_response,
    outline_output,
    search_output,
    section_output,
)

router = api_router("/v1/tools")


@router.post("/kb_search")
async def kb_search(
    body: KbSearchInput, caller: KbSearchCaller, service: ToolServiceDep, request: Request
) -> dict[str, Any]:
    filters = SearchFilters(
        file_ids=tuple(body.filters.file_ids or ()) if body.filters else (),
        doc_types=tuple(body.filters.doc_types or ()) if body.filters else (),
    )
    answer = await service.search(caller, TOOL_KB_SEARCH, body.query, filters)
    return success_body(to_json(search_output(answer)), request_id_of(request))


@router.post("/kb_outline")
async def kb_outline(
    body: KbOutlineInput, caller: KbOutlineCaller, service: ToolServiceDep, request: Request
) -> dict[str, Any]:
    name, nodes, truncated = await service.outline(caller, TOOL_KB_OUTLINE, body.file_id)
    return success_body(to_json(outline_output(body.file_id, name, nodes, truncated)), request_id_of(request))


@router.post("/kb_read_section")
async def kb_read_section(
    body: KbReadSectionInput, caller: KbReadSectionCaller, service: ToolServiceDep, request: Request
) -> dict[str, Any]:
    section = await service.read_section(caller, TOOL_KB_READ_SECTION, body.node_id)
    return success_body(to_json(section_output(section)), request_id_of(request))


@router.post("/kb_list_documents")
async def kb_list_documents(
    body: KbListDocumentsInput, caller: KbListDocumentsCaller, service: ToolServiceDep, request: Request
) -> dict[str, Any]:
    page, cursor, size = await service.list_documents(
        caller, TOOL_KB_LIST_DOCUMENTS, body.name_contains, body.doc_type, body.cursor, body.limit
    )
    return page_body([to_json(document_response(row)) for row in page], request_id_of(request), cursor, size)
