from typing import Any
from uuid import UUID

from fastapi import APIRouter, Request, Response, status

from thomas_ragx.api.services import KbServiceDep
from thomas_ragx.auth.dependencies import KbReadCaller, KbWriteCaller
from thomas_ragx.kb.dto import KnowledgeBaseResponse, KnowledgeBaseWrite, to_json
from thomas_ragx.platform.http_response import request_id_of, success_body

router = APIRouter(prefix="/v1/knowledge-bases")


@router.get("")
async def list_knowledge_bases(
    caller: KbReadCaller, service: KbServiceDep, request: Request
) -> dict[str, Any]:
    kbs = await service.list_all(caller)
    return success_body([to_json(KnowledgeBaseResponse.from_model(kb)) for kb in kbs], request_id_of(request))


@router.post("", status_code=status.HTTP_201_CREATED)
async def create_knowledge_base(
    body: KnowledgeBaseWrite, caller: KbWriteCaller, service: KbServiceDep, request: Request
) -> dict[str, Any]:
    kb = await service.create(caller, body.name)
    return success_body(to_json(KnowledgeBaseResponse.from_model(kb)), request_id_of(request))


@router.get("/{kb_id}")
async def get_knowledge_base(
    kb_id: UUID, caller: KbReadCaller, service: KbServiceDep, request: Request
) -> dict[str, Any]:
    kb = await service.get(caller, kb_id)
    return success_body(to_json(KnowledgeBaseResponse.from_model(kb)), request_id_of(request))


@router.patch("/{kb_id}")
async def rename_knowledge_base(
    kb_id: UUID, body: KnowledgeBaseWrite, caller: KbWriteCaller, service: KbServiceDep, request: Request
) -> dict[str, Any]:
    kb = await service.rename(caller, kb_id, body.name)
    return success_body(to_json(KnowledgeBaseResponse.from_model(kb)), request_id_of(request))


@router.delete("/{kb_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_knowledge_base(
    kb_id: UUID, caller: KbWriteCaller, service: KbServiceDep, request: Request
) -> Response:
    await service.delete(caller, kb_id, request_id_of(request))
    return Response(status_code=status.HTTP_204_NO_CONTENT)
