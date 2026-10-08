from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from time import perf_counter
from typing import Any, Final

import structlog
from fastapi import FastAPI, Request, Response
from opentelemetry.propagate import extract

from thomas_ragx.api.files import router as files_router
from thomas_ragx.api.health import router as health_router
from thomas_ragx.api.knowledge_bases import router as knowledge_bases_router
from thomas_ragx.api.manifest import router as manifest_router
from thomas_ragx.api.orgs import router as orgs_router
from thomas_ragx.auth.app_token import AppTokenVerifier, load_public_keys
from thomas_ragx.kb.file_service import FileService
from thomas_ragx.kb.service import KnowledgeBaseService
from thomas_ragx.platform.config import Config, load_config
from thomas_ragx.platform.constants import HTTP_STATUS_CLIENT_ERROR, HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import error_response, install_error_handlers
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.request_id import (
    REQUEST_ID_HEADER,
    REQUEST_ID_SOURCE_GENERATED,
    accept_request_id,
)
from thomas_ragx.platform.request_state import STATE_JTI, STATE_ORG_ID
from thomas_ragx.platform.tracing import (
    ATTR_ORG_ID,
    ATTR_REQUEST_ID,
    ATTR_SPAN_KIND,
    RAGX_TRACE_ID_HEADER,
    SPAN_KIND_CHAIN,
    configure_tracing,
    format_trace_id,
)
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.resources import Resources

MILLISECONDS_PER_SECOND: Final = 1000
ROUTE_UNMATCHED: Final = "unmatched"
ACCESS_LOG_EVENT: Final = "request completed"


def _route_pattern(request: Request) -> str:
    path = getattr(request.scope.get("route"), "path", None)
    return path if isinstance(path, str) else ROUTE_UNMATCHED


def _caller_fields(request: Request) -> dict[str, str]:
    fields: dict[str, str] = {}
    for name in (STATE_ORG_ID, STATE_JTI):
        value = getattr(request.state, name, None)
        if isinstance(value, str):
            fields[name] = value
    return fields


def _log_completed(logger: Any, status: int, fields: dict[str, object]) -> None:
    if status >= HTTP_STATUS_SERVER_ERROR:
        logger.error(ACCESS_LOG_EVENT, **fields)
    elif status >= HTTP_STATUS_CLIENT_ERROR:
        logger.warning(ACCESS_LOG_EVENT, **fields)
    else:
        logger.info(ACCESS_LOG_EVENT, **fields)


def _request_id_fields(sent: str | None, rejected: bool) -> dict[str, object]:
    if rejected:
        return {"request_id_source": REQUEST_ID_SOURCE_GENERATED, "client_request_id_rejected": True}
    if not sent:
        return {"request_id_source": REQUEST_ID_SOURCE_GENERATED}
    return {}


def create_app(config: Config | None = None, resources: Resources | None = None) -> FastAPI:
    config = config or load_config()
    resources = resources or Resources.from_config(config)
    app_token_verifier = AppTokenVerifier(
        load_public_keys(config.core_public_keys),
        issuer=config.app_token_issuer,
        audience=config.app_token_audience,
        leeway_seconds=config.app_token_leeway_seconds,
        max_lifetime_seconds=config.app_token_max_lifetime_seconds,
    )
    registry_service = create_registry_service(config, resources.pool)
    kb_service = KnowledgeBaseService(resources.pool, registry_service)
    file_service = FileService(
        resources.pool,
        resources.storage,
        max_upload_bytes=config.upload_max_bytes,
        multipart_overhead_bytes=config.upload_multipart_overhead_bytes,
        idempotency_ttl_seconds=config.idempotency_ttl_seconds,
        page_default_limit=config.page_default_limit,
        page_max_limit=config.page_max_limit,
    )
    configure_logging(config.log_level, config.service_name, config.environment)
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__)

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        await resources.open()
        try:
            yield
        finally:
            await resources.close()

    app = FastAPI(title="thomas-ragx", lifespan=lifespan)
    app.state.resources = resources
    app.state.app_token_verifier = app_token_verifier
    app.state.registry_service = registry_service
    app.state.kb_service = kb_service
    app.state.file_service = file_service
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
            caller = _caller_fields(request)
            if org_id := caller.get(STATE_ORG_ID):
                span.set_attribute(ATTR_ORG_ID, org_id)
            _log_completed(
                logger,
                response.status_code,
                {
                    "method": request.method,
                    "route": _route_pattern(request),
                    "status": response.status_code,
                    "duration_ms": round((perf_counter() - started) * MILLISECONDS_PER_SECOND),
                    **caller,
                    **_request_id_fields(sent, rejected),
                },
            )
        response.headers[REQUEST_ID_HEADER] = request_id
        response.headers[RAGX_TRACE_ID_HEADER] = format_trace_id(span.get_span_context().trace_id)
        return response

    app.include_router(health_router)
    app.include_router(orgs_router)
    app.include_router(knowledge_bases_router)
    app.include_router(files_router)
    app.include_router(manifest_router)
    return app
