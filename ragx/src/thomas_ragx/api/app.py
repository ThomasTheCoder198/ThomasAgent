import uuid
from collections.abc import Awaitable, Callable

import structlog
from fastapi import FastAPI, Request, Response
from opentelemetry import trace
from opentelemetry.propagate import extract

from thomas_ragx.api.health import router as health_router
from thomas_ragx.platform.config import Config, load_config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import error_response, install_error_handlers
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.tracing import configure_tracing

REQUEST_ID_HEADER = "X-Request-Id"


def create_app(config: Config | None = None) -> FastAPI:
    config = config or load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__) if provider else trace.get_tracer(__name__)
    app = FastAPI(title="thomas-ragx")
    app.state.tracer_provider = provider
    install_error_handlers(app)

    @app.middleware("http")
    async def request_id(request: Request, call_next: Callable[[Request], Awaitable[Response]]) -> Response:
        request_id = request.headers.get(REQUEST_ID_HEADER) or f"req_{uuid.uuid4()}"
        request.state.request_id = request_id
        with (
            tracer.start_as_current_span(
                "rag.http.request",
                context=extract(dict(request.headers)),
                record_exception=False,
                set_status_on_exception=False,
            ),
            structlog.contextvars.bound_contextvars(request_id=request_id),
        ):
            logger = structlog.get_logger(__name__)
            try:
                response = await call_next(request)
            except Exception as exc:
                # Exception messages may contain secrets, so only the type is recorded.
                logger.error("unhandled error", error_type=type(exc).__name__)  # noqa: TRY400
                response = error_response(request, AppError(ErrorCode.INTERNAL_ERROR))
            logger.info("request completed", method=request.method, status=response.status_code)
        response.headers[REQUEST_ID_HEADER] = request_id
        return response

    app.include_router(health_router)
    return app
