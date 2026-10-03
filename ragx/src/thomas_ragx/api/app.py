import uuid
from collections.abc import Awaitable, Callable

import structlog
from fastapi import FastAPI, Request, Response
from opentelemetry import trace
from opentelemetry.propagate import extract

from thomas_ragx.api.health import router as health_router
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError, error_response, install_error_handlers
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.settings import Settings, get_settings
from thomas_ragx.platform.tracing import configure_tracing

REQUEST_ID_HEADER = "X-Request-Id"


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or get_settings()
    configure_logging(settings.log_level, settings.service_name)
    provider = configure_tracing(settings)
    tracer = provider.get_tracer(__name__) if provider else trace.get_tracer(__name__)
    app = FastAPI(title="thomas-ragx")
    app.state.tracer_provider = provider
    install_error_handlers(app)

    @app.middleware("http")
    async def request_id(request: Request, call_next: Callable[[Request], Awaitable[Response]]) -> Response:
        rid = request.headers.get(REQUEST_ID_HEADER) or f"req_{uuid.uuid4()}"
        request.state.request_id = rid
        with (
            tracer.start_as_current_span(
                "rag.http.request",
                context=extract(dict(request.headers)),
                record_exception=False,
                set_status_on_exception=False,
            ),
            structlog.contextvars.bound_contextvars(request_id=rid, env=settings.env),
        ):
            logger = structlog.get_logger(__name__)
            try:
                response = await call_next(request)
            except Exception as exc:
                # Exception messages may contain secrets, so only the type is recorded.
                logger.error("unhandled error", error_type=type(exc).__name__)  # noqa: TRY400
                response = error_response(request, AppError(ErrorCode.INTERNAL_ERROR))
            logger.info("request completed", method=request.method, status=response.status_code)
        response.headers[REQUEST_ID_HEADER] = rid
        return response

    app.include_router(health_router)
    return app
