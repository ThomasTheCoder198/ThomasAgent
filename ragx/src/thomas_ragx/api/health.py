from typing import Any, Final

from fastapi import APIRouter, Request

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import request_id_of, success_body

STATUS_OK: Final = "ok"
STATUS_READY: Final = "ready"
router = APIRouter()


@router.get("/healthz")
def healthz(request: Request) -> dict[str, Any]:
    return success_body({"status": STATUS_OK}, request_id_of(request))


@router.get("/readyz")
async def readyz(request: Request) -> dict[str, Any]:
    checks = await request.app.state.resources.readiness()
    if not all(checks.values()):
        # The failing dependency is logged by Resources; the body never names internals.
        raise AppError(ErrorCode.PROVIDER_UNAVAILABLE)
    return success_body({"status": STATUS_READY}, request_id_of(request))
