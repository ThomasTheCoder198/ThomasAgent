from collections import Counter
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any, Final, cast
from urllib.parse import urlencode
from uuid import UUID

import structlog
from fastapi import APIRouter, FastAPI, Request, Response, status
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from jinja2 import Environment, PackageLoader, select_autoescape
from pydantic import SecretStr
from starlette.datastructures import FormData

from thomas_ragx.admin.model import AdminSession
from thomas_ragx.admin.service import AdminService
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.constants import HTTP_STATUS_SERVER_ERROR
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import request_id_of
from thomas_ragx.platform.request_id import is_valid_request_id
from thomas_ragx.registry.listing import LISTING_ROUTES, ListingMethod
from thomas_ragx.registry.model import ROLE_CAPABILITIES, Capability, ModelRecord, ProviderKind, Role
from thomas_ragx.registry.service import RegistryService
from thomas_ragx.ui.i18n import LANG_COOKIE, LANG_VALUES, error_text, translator, ui_language
from thomas_ragx.ui.session import (
    LOGIN_PATH,
    SESSION_COOKIE,
    STATE_UI_SESSION,
    UI_COOKIE_PATH,
    AdminDep,
    CsrfAdminDep,
    LoginRequiredError,
)

STATIC_PATH: Final = "/ui/static"
HOME_PATH: Final = "/ui/providers"
PROVIDERS_PATH: Final = "/ui/providers"
MODELS_PATH: Final = "/ui/models"
ROLES_PATH: Final = "/ui/roles"
LANG_COOKIE_MAX_AGE_SECONDS: Final = 31_536_000
NOTICES: Final = frozenset({"saved", "deleted", "synced"})
QUERY_ERROR: Final = "error"
QUERY_REQUEST_ID: Final = "rid"
QUERY_NOTICE: Final = "notice"
LISTABLE_KINDS: Final = frozenset(
    kind for kind, route in LISTING_ROUTES.items() if route.method is not ListingMethod.MANUAL
)
# 'self' only: htmx, the theme script, CSS and fonts are all served from /ui/static.
SECURITY_HEADERS: Final = {
    "Content-Security-Policy": (
        "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; "
        "form-action 'self'; frame-ancestors 'none'; base-uri 'none'"
    ),
    "X-Content-Type-Options": "nosniff",
    "Referrer-Policy": "same-origin",
    "Cache-Control": "no-store",
}
TEMPLATES: Final = Jinja2Templates(
    env=Environment(
        loader=PackageLoader("thomas_ragx.ui", "templates"), autoescape=select_autoescape(("html",))
    )
)
router = APIRouter(prefix="/ui", include_in_schema=False)
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class UiSettings:
    session_ttl_seconds: int
    cookie_secure: bool


def _registry(request: Request) -> RegistryService:
    return cast(RegistryService, request.app.state.registry_service)


def _admin(request: Request) -> AdminService:
    return cast(AdminService, request.app.state.admin_service)


def _shown_code(exc: AppError) -> ErrorCode:
    """Logs a server-side failure once, at the UI boundary, and hides its cause like the JSON API does."""
    if exc.http_status() < HTTP_STATUS_SERVER_ERROR:
        return exc.code
    log.error("ui action failed", code=exc.code.value)
    return ErrorCode.INTERNAL_ERROR


def _shown_request_id(request: Request) -> str:
    # After post/redirect/get the failing POST's id travels in ?rid, because that is the id in the logs.
    carried = request.query_params.get(QUERY_REQUEST_ID)
    return carried if is_valid_request_id(carried) else request_id_of(request)


def render(
    request: Request,
    template: str,
    page: str,
    context: dict[str, Any] | None = None,
    *,
    status_code: int = status.HTTP_200_OK,
    error_code: str | None = None,
) -> HTMLResponse:
    lang = ui_language(request)
    session: AdminSession | None = getattr(request.state, STATE_UI_SESSION, None)
    code = error_code or request.query_params.get(QUERY_ERROR)
    error = error_text(code, lang) if code else None
    notice = request.query_params.get(QUERY_NOTICE)
    values = {
        "t": translator(lang),
        "lang": lang.value,
        "page": page,
        "admin": session,
        "csrf_token": session.csrf_token if session else "",
        "error": error,
        "request_id": _shown_request_id(request) if error else "",
        "notice": notice if notice in NOTICES else None,
        "path": request.url.path,
        **(context or {}),
    }
    return TEMPLATES.TemplateResponse(
        request, template, values, status_code=status_code, headers=SECURITY_HEADERS
    )


def _redirect(path: str, **query: str) -> RedirectResponse:
    target = f"{path}?{urlencode(query)}" if query else path
    return RedirectResponse(target, status_code=status.HTTP_303_SEE_OTHER, headers=SECURITY_HEADERS)


async def _act(
    request: Request, path: str, notice: str, action: Callable[[], Awaitable[object]]
) -> RedirectResponse:
    """Post/redirect/get: a catalog error returns as ?error=CODE&rid=<this request>, shown localized."""
    try:
        await action()
    except AppError as exc:
        return _redirect(
            path, **{QUERY_ERROR: _shown_code(exc).value, QUERY_REQUEST_ID: request_id_of(request)}
        )
    return _redirect(path, **{QUERY_NOTICE: notice})


def _field_error(field: str) -> AppError:
    return AppError(ErrorCode.VALIDATION_FAILED, details={"fields": [{"field": field, "code": "INVALID"}]})


def _text(form: FormData, name: str) -> str:
    value = form.get(name)
    return value.strip() if isinstance(value, str) else ""


def _secret(form: FormData, name: str) -> SecretStr | None:
    value = _text(form, name)
    return SecretStr(value) if value else None


def _positive_int(form: FormData, name: str) -> int | None:
    value = _text(form, name)
    if not value:
        return None
    try:
        number = int(value)
    except ValueError:
        raise _field_error(name) from None
    if number < 1:
        raise _field_error(name)
    return number


def _uuid(form: FormData, name: str) -> UUID:
    try:
        return UUID(_text(form, name))
    except ValueError:
        raise _field_error(name) from None


def _kind(form: FormData) -> ProviderKind:
    try:
        return ProviderKind(_text(form, "kind"))
    except ValueError:
        raise _field_error("kind") from None


def _capabilities(form: FormData) -> frozenset[Capability]:
    try:
        return frozenset(Capability(str(value)) for value in form.getlist("capabilities"))
    except ValueError:
        raise _field_error("capabilities") from None


def _role_options(models: list[ModelRecord]) -> dict[Role, list[ModelRecord]]:
    return {
        role: [
            model
            for model in models
            if model.capabilities & ROLE_CAPABILITIES[role]
            and (role is not Role.EMBEDDING or model.embedding_dims)
        ]
        for role in Role
    }


@router.get("")
async def home() -> RedirectResponse:
    return _redirect(HOME_PATH)


@router.get("/login")
async def login_page(request: Request) -> HTMLResponse:
    return render(request, "login.html", "login")


@router.post("/login")
async def login_submit(request: Request) -> Response:
    form = await request.form()
    email = _text(form, "email")
    ip = request.client.host if request.client else ""
    try:
        token, _ = await _admin(request).login(email, str(form.get("password") or ""), ip)
    except AppError as exc:
        return render(
            request,
            "login.html",
            "login",
            {"email": email},
            status_code=exc.http_status(),
            error_code=_shown_code(exc).value,
        )
    settings = cast(UiSettings, request.app.state.ui_settings)
    response = _redirect(HOME_PATH)
    response.set_cookie(
        SESSION_COOKIE,
        token,
        max_age=settings.session_ttl_seconds,
        httponly=True,
        samesite="lax",
        secure=settings.cookie_secure,
        path=UI_COOKIE_PATH,
    )
    return response


@router.post("/logout")
async def logout(request: Request, _: CsrfAdminDep) -> RedirectResponse:
    token = request.cookies.get(SESSION_COOKIE)
    if token:
        await _admin(request).logout(token)
    response = _redirect(LOGIN_PATH)
    response.delete_cookie(SESSION_COOKIE, path=UI_COOKIE_PATH)
    return response


@router.post("/lang")
async def set_language(request: Request) -> RedirectResponse:
    form = await request.form()
    back = _text(form, "next")
    # Only same-UI paths: "next" must never become an open redirect.
    target = back if back.startswith("/ui/") and not back.startswith("//") else HOME_PATH
    response = _redirect(target)
    choice = _text(form, "lang")
    if choice in LANG_VALUES:
        response.set_cookie(
            LANG_COOKIE,
            choice,
            max_age=LANG_COOKIE_MAX_AGE_SECONDS,
            httponly=True,
            samesite="lax",
            path=UI_COOKIE_PATH,
        )
    return response


@router.get("/providers")
async def providers_page(request: Request, _: AdminDep) -> HTMLResponse:
    registry = _registry(request)
    providers = await registry.list_providers()
    # The delete dialog names how many models go with the provider.
    model_counts = Counter(model.provider_id for model in await registry.list_models())
    return render(
        request,
        "providers.html",
        "providers",
        {
            "providers": providers,
            "kinds": list(ProviderKind),
            "listable": LISTABLE_KINDS,
            "model_counts": model_counts,
        },
    )


@router.post("/providers")
async def create_provider(request: Request, _: CsrfAdminDep) -> RedirectResponse:
    form = await request.form()

    async def action() -> object:
        return await _registry(request).create_provider(
            _kind(form), _text(form, "name"), _text(form, "base_url"), _secret(form, "api_key")
        )

    return await _act(request, PROVIDERS_PATH, "saved", action)


@router.post("/providers/{provider_id}")
async def update_provider(request: Request, provider_id: UUID, _: CsrfAdminDep) -> RedirectResponse:
    form = await request.form()

    async def action() -> object:
        return await _registry(request).update_provider(
            provider_id,
            _text(form, "name"),
            _text(form, "base_url"),
            _secret(form, "api_key"),
            clear_api_key=bool(_text(form, "clear_api_key")),
        )

    return await _act(request, PROVIDERS_PATH, "saved", action)


@router.post("/providers/{provider_id}/delete")
async def delete_provider(request: Request, provider_id: UUID, _: CsrfAdminDep) -> RedirectResponse:
    return await _act(
        request, PROVIDERS_PATH, "deleted", lambda: _registry(request).delete_provider(provider_id)
    )


@router.post("/providers/{provider_id}/sync")
async def sync_provider(request: Request, provider_id: UUID, _: CsrfAdminDep) -> RedirectResponse:
    return await _act(request, MODELS_PATH, "synced", lambda: _registry(request).sync_models(provider_id))


@router.get("/models")
async def models_page(request: Request, _: AdminDep, provider: UUID | None = None) -> HTMLResponse:
    registry = _registry(request)
    providers = await registry.list_providers()
    return render(
        request,
        "models.html",
        "models",
        {
            "providers": providers,
            "provider_names": {p.id: p.name for p in providers},
            "models": await registry.list_models(provider),
            "selected_provider": provider,
            "capabilities": list(Capability),
        },
    )


@router.post("/models")
async def add_model(request: Request, _: CsrfAdminDep) -> RedirectResponse:
    form = await request.form()

    async def action() -> object:
        return await _registry(request).add_manual_model(
            _uuid(form, "provider_id"),
            _text(form, "name"),
            _capabilities(form),
            _positive_int(form, "embedding_dims"),
            _positive_int(form, "context_window"),
        )

    return await _act(request, MODELS_PATH, "saved", action)


@router.post("/models/{model_id}")
async def update_model(request: Request, model_id: UUID, _: CsrfAdminDep) -> RedirectResponse:
    form = await request.form()

    async def action() -> object:
        return await _registry(request).update_model(
            model_id, _capabilities(form), _positive_int(form, "embedding_dims")
        )

    return await _act(request, MODELS_PATH, "saved", action)


@router.post("/models/{model_id}/delete")
async def delete_model(request: Request, model_id: UUID, _: CsrfAdminDep) -> RedirectResponse:
    return await _act(request, MODELS_PATH, "deleted", lambda: _registry(request).delete_model(model_id))


@router.post("/models/{model_id}/test")
async def test_model(request: Request, model_id: UUID, _: CsrfAdminDep) -> HTMLResponse:
    lang = ui_language(request)
    try:
        await _registry(request).test_model(model_id)
    except AppError as exc:
        message = error_text(_shown_code(exc).value, lang)
        context = {"ok": False, "message": message, "test_request_id": request_id_of(request)}
        return render(request, "_test_result.html", "models", context)
    return render(
        request, "_test_result.html", "models", {"ok": True, "message": translator(lang)("models.test_ok")}
    )


@router.get("/roles")
async def roles_page(request: Request, _: AdminDep) -> HTMLResponse:
    registry = _registry(request)
    return render(
        request,
        "roles.html",
        "roles",
        {
            "roles": list(Role),
            "assigned": await registry.roles(),
            "options": _role_options(await registry.list_models()),
            "role_capabilities": ROLE_CAPABILITIES,
        },
    )


@router.post("/roles")
async def assign_role(request: Request, _: CsrfAdminDep) -> RedirectResponse:
    form = await request.form()

    async def action() -> object:
        try:
            role = Role(_text(form, "role"))
        except ValueError:
            raise _field_error("role") from None
        if _text(form, "model_id"):
            await _registry(request).assign_role(role, _uuid(form, "model_id"))
        else:
            await _registry(request).clear_role(role)
        return None

    return await _act(request, ROLES_PATH, "saved", action)


def install_ui(app: FastAPI, config: Config, admin_service: AdminService) -> None:
    app.state.admin_service = admin_service
    app.state.ui_settings = UiSettings(config.ui_session_ttl_seconds, config.ui_cookie_secure)
    # Packaged assets: served from the installed wheel, so the image needs no extra copy step.
    app.mount(STATIC_PATH, StaticFiles(packages=[("thomas_ragx.ui", "static")]), name="ui_static")
    app.include_router(router)
    api_error_handler = cast(
        Callable[[Request, AppError], Awaitable[Response]], app.exception_handlers[AppError]
    )

    @app.exception_handler(LoginRequiredError)
    async def _login_required(_: Request, __: LoginRequiredError) -> RedirectResponse:
        return _redirect(LOGIN_PATH)

    @app.exception_handler(AppError)
    async def _app_error(request: Request, exc: AppError) -> Response:
        # A browser on /ui gets a page with the localized message and requestId, never a JSON body.
        if not request.url.path.startswith(UI_COOKIE_PATH + "/"):
            return await api_error_handler(request, exc)
        return render(
            request, "error.html", "error", status_code=exc.http_status(), error_code=_shown_code(exc).value
        )
