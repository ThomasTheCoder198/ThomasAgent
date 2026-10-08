from typing import Any, Final
from urllib.parse import quote
from uuid import UUID

from fastapi import APIRouter, Request, Response, status
from fastapi.responses import JSONResponse, StreamingResponse

from thomas_ragx.api.services import FileServiceDep
from thomas_ragx.auth.dependencies import KbReadCaller, KbWriteCaller
from thomas_ragx.kb.dto import FileResponse, to_json
from thomas_ragx.kb.upload import receive_upload
from thomas_ragx.platform.http_response import page_body, request_id_of, success_body

IDEMPOTENCY_KEY_HEADER: Final = "Idempotency-Key"
PREVIEW_MEDIA_TYPE: Final = "application/json"
router = APIRouter(prefix="/v1/knowledge-bases/{kb_id}/files")


def content_disposition(name: str) -> str:
    # RFC 5987 keeps Vietnamese names intact; no ASCII fallback is sent because every caller is core.
    return f"attachment; filename*=UTF-8''{quote(name, safe='')}"


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


@router.post("")
async def upload_file(
    kb_id: UUID, caller: KbWriteCaller, service: FileServiceDep, request: Request
) -> JSONResponse:
    await service.ensure_kb(caller, kb_id)
    async with receive_upload(
        request, service.max_upload_bytes, service.multipart_overhead_bytes
    ) as received:
        stored, created = await service.upload(
            caller, kb_id, received, request.headers.get(IDEMPOTENCY_KEY_HEADER), request_id_of(request)
        )
    return JSONResponse(
        status_code=status.HTTP_201_CREATED if created else status.HTTP_200_OK,
        content=success_body(to_json(FileResponse.from_model(stored)), request_id_of(request)),
    )


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


@router.get("/{file_id}/raw")
async def download_file(
    kb_id: UUID, file_id: UUID, caller: KbReadCaller, service: FileServiceDep
) -> StreamingResponse:
    file, chunks = await service.open_raw(caller, kb_id, file_id)
    headers = {"Content-Disposition": content_disposition(file.name), "Content-Length": str(file.size_bytes)}
    return StreamingResponse(chunks, media_type=file.content_type, headers=headers)


@router.get("/{file_id}/preview")
async def get_preview(
    kb_id: UUID, file_id: UUID, caller: KbReadCaller, service: FileServiceDep
) -> StreamingResponse:
    return StreamingResponse(
        await service.open_preview(caller, kb_id, file_id), media_type=PREVIEW_MEDIA_TYPE
    )
