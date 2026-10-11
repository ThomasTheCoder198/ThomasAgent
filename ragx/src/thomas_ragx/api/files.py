from typing import Any, Final
from uuid import UUID

from fastapi import Request, Response, status
from fastapi.responses import StreamingResponse

from thomas_ragx.api.routing import api_router
from thomas_ragx.api.services import FileServiceDep
from thomas_ragx.auth.dependencies import KbReadCaller, KbWriteCaller
from thomas_ragx.kb.dto import DownloadUrlResponse, FileResponse, UploadRequest, UploadTicketResponse, to_json
from thomas_ragx.platform.http_response import page_body, request_id_of, success_body

IDEMPOTENCY_KEY_HEADER: Final = "Idempotency-Key"
PREVIEW_MEDIA_TYPE: Final = "application/json"
router = api_router("/v1/knowledge-bases/{kb_id}/files")
uploads_router = api_router("/v1/knowledge-bases/{kb_id}/uploads")


@router.get("")
async def list_files(
    kb_id: UUID,
    caller: KbReadCaller,
    service: FileServiceDep,
    request: Request,
    cursor: str | None = None,
    limit: int | None = None,
) -> dict[str, Any]:
    page, next_cursor, size = await service.list_page(caller, kb_id, cursor, limit)
    return page_body(
        [to_json(FileResponse.from_model(f)) for f in page], request_id_of(request), next_cursor, size
    )


@uploads_router.post("", status_code=status.HTTP_201_CREATED)
async def create_upload(
    kb_id: UUID, body: UploadRequest, caller: KbWriteCaller, service: FileServiceDep, request: Request
) -> dict[str, Any]:
    ticket = await service.create_upload(caller, kb_id, body.name, body.size_bytes)
    return success_body(to_json(UploadTicketResponse.from_model(ticket)), request_id_of(request))


@router.post("/{file_id}/confirm")
async def confirm_upload(
    kb_id: UUID, file_id: UUID, caller: KbWriteCaller, service: FileServiceDep, request: Request
) -> dict[str, Any]:
    stored = await service.confirm_upload(
        caller, kb_id, file_id, request.headers.get(IDEMPOTENCY_KEY_HEADER), request_id_of(request)
    )
    return success_body(to_json(FileResponse.from_model(stored)), request_id_of(request))


@router.delete("/{file_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_file(
    kb_id: UUID, file_id: UUID, caller: KbWriteCaller, service: FileServiceDep, request: Request
) -> Response:
    await service.delete(caller, kb_id, file_id, request_id_of(request))
    return Response(status_code=status.HTTP_204_NO_CONTENT)


@router.post("/{file_id}/retry")
async def retry_file(
    kb_id: UUID, file_id: UUID, caller: KbWriteCaller, service: FileServiceDep, request: Request
) -> dict[str, Any]:
    file = await service.retry(caller, kb_id, file_id, request_id_of(request))
    return success_body(to_json(FileResponse.from_model(file)), request_id_of(request))


@router.get("/{file_id}/download-url")
async def get_download_url(
    kb_id: UUID, file_id: UUID, caller: KbReadCaller, service: FileServiceDep, request: Request
) -> dict[str, Any]:
    link = await service.download_url(caller, kb_id, file_id)
    return success_body(to_json(DownloadUrlResponse.from_model(link)), request_id_of(request))


@router.get("/{file_id}/preview")
async def get_preview(
    kb_id: UUID, file_id: UUID, caller: KbReadCaller, service: FileServiceDep
) -> StreamingResponse:
    return StreamingResponse(
        await service.open_preview(caller, kb_id, file_id), media_type=PREVIEW_MEDIA_TYPE
    )
