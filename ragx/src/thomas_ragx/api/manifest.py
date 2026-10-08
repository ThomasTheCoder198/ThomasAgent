from importlib.metadata import version
from typing import Any, Final

from fastapi import APIRouter, Request

from thomas_ragx.auth.dependencies import KbReadCaller
from thomas_ragx.platform.http_response import request_id_of, success_body

APP_NAME: Final = "ragx"
DISTRIBUTION: Final = "thomas-ragx"
router = APIRouter(prefix="/v1")


@router.get("/manifest")
async def get_manifest(_: KbReadCaller, request: Request) -> dict[str, Any]:
    # Tool entries (closed input schemas, timeoutMs, failure_mode) arrive with the tool routes in M1.3.
    return success_body(
        {"name": APP_NAME, "version": version(DISTRIBUTION), "tools": []}, request_id_of(request)
    )
