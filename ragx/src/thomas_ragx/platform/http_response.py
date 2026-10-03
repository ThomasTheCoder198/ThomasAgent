from typing import Any

import structlog
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from opentelemetry import trace
from starlette.exceptions import HTTPException as StarletteHTTPException

from thomas_ragx.platform.constants import HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError, lang_from_header

_STATUS_TO_CODE = {
    404: ErrorCode.NOT_FOUND,
    405: ErrorCode.METHOD_NOT_ALLOWED,
    413: ErrorCode.PAYLOAD_TOO_LARGE,
}

log = structlog.get_logger(__name__)


def _trace_id() -> str | None:
    ctx = trace.get_current_span().get_span_context()
    return format(ctx.trace_id, "032x") if ctx.is_valid else None


def request_id_of(request: Request) -> str:
    return str(getattr(request.state, "request_id", ""))


def success_body(data: Any, request_id: str) -> dict[str, Any]:
    return {"data": data, "meta": {"requestId": request_id}}


def error_response(request: Request, err: AppError) -> JSONResponse:
    status = err.http_status()
    if status >= HTTP_STATUS_SERVER_ERROR:
        err = AppError(ErrorCode.INTERNAL_ERROR)
    body: dict[str, Any] = {
        "code": err.code.value,
        "message": err.localized_message(lang_from_header(request.headers.get("accept-language"))),
    }
    if err.details is not None and status < HTTP_STATUS_SERVER_ERROR:
        body["details"] = err.details
    if trace_id := _trace_id():
        body["traceId"] = trace_id
    return JSONResponse(status_code=status, content={"error": body})


def install_error_handlers(app: FastAPI) -> None:
    @app.exception_handler(AppError)
    async def _app_error(request: Request, exc: AppError) -> JSONResponse:
        if exc.http_status() >= HTTP_STATUS_SERVER_ERROR:
            log.error(
                "request failed", code=exc.code.value, error=str(exc), request_id=request_id_of(request)
            )
        return error_response(request, exc)

    @app.exception_handler(RequestValidationError)
    async def _validation(request: Request, exc: RequestValidationError) -> JSONResponse:
        fields = [{"field": str(e["loc"][-1]), "code": str(e["type"]).upper()} for e in exc.errors()]
        return error_response(request, AppError(ErrorCode.VALIDATION_FAILED, details={"fields": fields}))

    @app.exception_handler(StarletteHTTPException)
    async def _http(request: Request, exc: StarletteHTTPException) -> JSONResponse:
        return error_response(
            request, AppError(_STATUS_TO_CODE.get(exc.status_code, ErrorCode.INTERNAL_ERROR))
        )

    @app.exception_handler(Exception)
    async def _unexpected(request: Request, exc: Exception) -> JSONResponse:
        log.error("unhandled error", error_type=type(exc).__name__, request_id=request_id_of(request))
        return error_response(request, AppError(ErrorCode.INTERNAL_ERROR))
