from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from pathlib import Path
from time import perf_counter
from typing import Any, Final

import structlog
from fastapi import FastAPI, Request, Response
from opentelemetry.propagate import extract
from opentelemetry.trace import StatusCode

from thomas_ragx.admin.limiter import LoginLimiter
from thomas_ragx.admin.service import AdminService
from thomas_ragx.api.files import router as files_router
from thomas_ragx.api.files import uploads_router
from thomas_ragx.api.health import router as health_router
from thomas_ragx.api.knowledge_bases import router as knowledge_bases_router
from thomas_ragx.api.manifest import router as manifest_router
from thomas_ragx.api.orgs import router as orgs_router
from thomas_ragx.api.tools import router as tools_router
from thomas_ragx.auth.app_token import AppTokenVerifier, load_public_keys
from thomas_ragx.ingest.indexing import BM25_DIR_NAME
from thomas_ragx.kb.file_service import FileService
from thomas_ragx.kb.presign import create_url_signer
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
from thomas_ragx.platform.request_state import STATE_JTI, STATE_ORG_ID, STATE_TOOL
from thomas_ragx.platform.retry import retrying
from thomas_ragx.platform.tracing import (
    ATTR_ORG_ID,
    ATTR_REQUEST_ID,
    ATTR_SPAN_KIND,
    ATTR_TOOL,
    RAGX_TRACE_ID_HEADER,
    SPAN_KIND_CHAIN,
    configure_tracing,
    format_trace_id,
)
from thomas_ragx.providers.embedding import LiteLLMEmbedder
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.resources import Resources
from thomas_ragx.retrieval.qdrant_store import QdrantStore
from thomas_ragx.retrieval.rerank import LiteLLMReranker
from thomas_ragx.retrieval.search_service import SearchService
from thomas_ragx.retrieval.segmenter import create_segmenter
from thomas_ragx.retrieval.sparse import SparseEncoder
from thomas_ragx.retrieval.tool_catalog import manifest_tools
from thomas_ragx.retrieval.tool_service import KbToolService
from thomas_ragx.ui.routes import install_ui
from thomas_ragx.worker.metrics import (
    HTTP_DURATION_NAME,
    HTTP_REQUESTS_NAME,
    configure_worker_metrics,
)

MILLISECONDS_PER_SECOND: Final = 1000
ROUTE_UNMATCHED: Final = "unmatched"
ACCESS_LOG_EVENT: Final = "request completed"
HTTP_METRIC_METHOD: Final = "method"
HTTP_METRIC_ROUTE: Final = "route"
HTTP_METRIC_STATUS: Final = "status"


def _route_pattern(request: Request) -> str:
    path = getattr(request.scope.get("route"), "path", None)
    return path if isinstance(path, str) else ROUTE_UNMATCHED


def _caller_fields(request: Request) -> dict[str, str]:
    fields: dict[str, str] = {}
    for name in (STATE_ORG_ID, STATE_JTI, STATE_TOOL):
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
        create_url_signer(config),
        max_upload_bytes=config.upload_max_bytes,
        upload_url_ttl=config.upload_url_ttl,
        download_url_ttl=config.download_url_ttl,
        idempotency_ttl_seconds=config.idempotency_ttl_seconds,
        page_default_limit=config.page_default_limit,
        page_max_limit=config.page_max_limit,
    )
    admin_service = AdminService(
        resources.pool,
        LoginLimiter(resources.redis, config.ui_login_max_attempts, config.ui_login_window_seconds),
        config.ui_session_ttl_seconds,
    )
    configure_logging(config.log_level, config.service_name, config.environment)
    provider = configure_tracing(config)
    tracer = provider.get_tracer(__name__)
    metrics_provider, metrics = configure_worker_metrics(config)
    meter = metrics_provider.get_meter(__name__)
    http_requests = meter.create_counter(HTTP_REQUESTS_NAME)
    http_duration = meter.create_histogram(HTTP_DURATION_NAME, unit="s")

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        await resources.open()
        try:
            yield
        finally:
            await resources.close()
            metrics_provider.shutdown()

    app = FastAPI(title="thomas-ragx", lifespan=lifespan)
    app.state.resources = resources
    app.state.app_token_verifier = app_token_verifier
    app.state.registry_service = registry_service
    app.state.kb_service = kb_service
    app.state.file_service = file_service
    app.state.tracer_provider = provider
    app.state.metrics_provider = metrics_provider
    search_service = SearchService(
        resources.pool,
        registry_service,
        LiteLLMEmbedder(lambda: retrying(config), metrics=metrics),
        SparseEncoder(
            create_segmenter(config.vi_tokenizer),
            Path(config.work_dir) / BM25_DIR_NAME,
            k=config.bm25_k,
            b=config.bm25_b,
            avg_len=config.bm25_avg_len,
        ),
        QdrantStore(resources.qdrant, upsert_batch_size=config.qdrant_upsert_batch_size),
        LiteLLMReranker(lambda: retrying(config), metrics=metrics),
        tracer,
        dense_limit=config.search_dense_limit,
        sparse_limit=config.search_sparse_limit,
        fused_limit=config.search_fused_limit,
        result_limit=config.search_result_limit,
        min_score=config.search_min_rerank_score,
        trace_content=config.trace_content,
    )
    app.state.tool_service = KbToolService(
        resources.pool,
        search_service,
        tracer,
        timeout_seconds=config.tool_timeout_ms / MILLISECONDS_PER_SECOND,
        section_max_chars=config.section_max_chars,
        outline_max_nodes=config.outline_max_nodes,
        page_default_limit=config.page_default_limit,
        page_max_limit=config.page_max_limit,
    )
    app.state.manifest_tools = manifest_tools(config.tool_timeout_ms)
    app.include_router(tools_router)

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
            duration_seconds = perf_counter() - started
            http_attributes = {
                HTTP_METRIC_METHOD: request.method,
                HTTP_METRIC_ROUTE: _route_pattern(request),
                HTTP_METRIC_STATUS: response.status_code,
            }
            http_requests.add(1, http_attributes)
            http_duration.record(duration_seconds, http_attributes)
            if response.status_code >= HTTP_STATUS_SERVER_ERROR:
                span.set_status(StatusCode.ERROR)
            caller = _caller_fields(request)
            if org_id := caller.get(STATE_ORG_ID):
                span.set_attribute(ATTR_ORG_ID, org_id)
            if tool := caller.get(STATE_TOOL):
                span.set_attribute(ATTR_TOOL, tool)
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
    app.include_router(uploads_router)
    app.include_router(files_router)
    app.include_router(manifest_router)
    install_ui(app, config, admin_service)
    return app
