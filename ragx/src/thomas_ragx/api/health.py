from typing import Any

from fastapi import APIRouter, Request

from thomas_ragx.platform.http_response import request_id_of, success_body

router = APIRouter()


@router.get("/healthz")
def healthz(request: Request) -> dict[str, Any]:
    return success_body({"status": "ok"}, request_id_of(request))
