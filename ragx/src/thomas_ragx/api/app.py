from collections.abc import Awaitable, Callable
from time import perf_counter
from typing import Final

import structlog
from fastapi import FastAPI, Request, Response
from opentelemetry.propagate import extract

from thomas_ragx.api.health import router as health_router
from thomas_ragx.platform.config import Config, load_config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import error_response, install_error_handlers
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.request_id import (
    REQUEST_ID_HEADER,
    REQUEST_ID_SOURCE_GENERATED,
    accept_request_id,
)
from thomas_ragx.platform.tracing import (
    ATTR_REQUEST_ID,
    ATTR_SPAN_KIND,
    RAGX_TRACE_ID_HEADER,
    SPAN_KIND_CHAIN,
    configure_tracing,
    format_trace_id,
)

MILLISECONDS_PER_SECOND: Final = 1000


def _request_id_fields(sent: str | None, rejected: bool) -> dict[str, object]:
    if rejected:
        return {"request_id_source": REQUEST_ID_SOURCE_GENERATED, "client_request_id_rejected": True}
    if not sent:
        return {"request_id_source": REQUEST_ID_SOURCE_GENERATED}
    return {}


def create_app(config: Config | None = None) -> FastAPI:
    config = config or load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__)
    app = FastAPI(title="thomas-ragx")
    app.state.tracer_provider = provider
    install_error_handlers(app)

    @app.middleware("http")
    async def request_id(request: Request, call_next: Callable[[Request], Awaitable[Response]]) -> Response:
        started = perf_counter()
        sent = request.headers.get(REQUEST_ID_HEADER)
        request_id, rejected = accept_request_id(sent)
        request.state.request_id = request_id
        with (
            tracer.start_as_current_span(
                "rag.http.request",
                context=extract(dict(request.headers)),
                record_exception=False,
                set_status_on_exception=False,
                attributes={ATTR_REQUEST_ID: request_id, ATTR_SPAN_KIND: SPAN_KIND_CHAIN},
            ) as span,
            structlog.contextvars.bound_contextvars(request_id=request_id),
        ):
            logger = structlog.get_logger(__name__)
            try:
                response = await call_next(request)
            except Exception as exc:
                # Exception messages may contain secrets, so only the type is recorded.
                logger.error("unhandled error", error_type=type(exc).__name__)  # noqa: TRY400
                response = error_response(request, AppError(ErrorCode.INTERNAL_ERROR))
            logger.info(
                "request completed",
                method=request.method,
                status=response.status_code,
                duration_ms=(perf_counter() - started) * MILLISECONDS_PER_SECOND,
                **_request_id_fields(sent, rejected),
            )
        response.headers[REQUEST_ID_HEADER] = request_id
        response.headers[RAGX_TRACE_ID_HEADER] = format_trace_id(span.get_span_context().trace_id)
        return response

    app.include_router(health_router)
    return app
