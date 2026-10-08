from typing import Annotated, cast

from fastapi import Depends, Request

from thomas_ragx.kb.file_service import FileService
from thomas_ragx.kb.service import KnowledgeBaseService


def get_kb_service(request: Request) -> KnowledgeBaseService:
    return cast(KnowledgeBaseService, request.app.state.kb_service)


KbServiceDep = Annotated[KnowledgeBaseService, Depends(get_kb_service)]


def get_file_service(request: Request) -> FileService:
    return cast(FileService, request.app.state.file_service)


FileServiceDep = Annotated[FileService, Depends(get_file_service)]
