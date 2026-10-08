import secrets
from typing import Annotated, Final, cast

from fastapi import Depends, Request

from thomas_ragx.admin.model import AdminSession
from thomas_ragx.admin.service import AdminService
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

SESSION_COOKIE: Final = "ragx_session"
CSRF_FIELD: Final = "csrf_token"
CSRF_HEADER: Final = "x-csrf-token"
STATE_UI_SESSION: Final = "ui_session"
UI_COOKIE_PATH: Final = "/ui"
LOGIN_PATH: Final = "/ui/login"


class LoginRequiredError(Exception):
    """Raised by require_admin; install_ui turns it into a redirect to the login page."""


async def require_admin(request: Request) -> AdminSession:
    token = request.cookies.get(SESSION_COOKIE)
    service = cast(AdminService, request.app.state.admin_service)
    session = await service.session(token) if token else None
    if session is None:
        raise LoginRequiredError
    setattr(request.state, STATE_UI_SESSION, session)
    return session


AdminDep = Annotated[AdminSession, Depends(require_admin)]


async def require_csrf(request: Request, session: AdminDep) -> AdminSession:
    sent = request.headers.get(CSRF_HEADER) or str((await request.form()).get(CSRF_FIELD) or "")
    if not secrets.compare_digest(sent, session.csrf_token):
        raise AppError(ErrorCode.AUTH_CSRF_INVALID)
    return session


CsrfAdminDep = Annotated[AdminSession, Depends(require_csrf)]
