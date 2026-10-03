from typing import Any

from fastapi import APIRouter, Request

from thomas_ragx.platform.errors import data_envelope, request_id

router = APIRouter()


@router.get("/healthz")
def healthz(request: Request) -> dict[str, Any]:
    return data_envelope({"status": "ok"}, request_id(request))
