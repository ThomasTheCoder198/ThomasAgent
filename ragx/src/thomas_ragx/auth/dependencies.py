from collections.abc import Awaitable, Callable
from typing import Annotated, Final

import structlog
from fastapi import Depends, Request

from thomas_ragx.auth.app_token import (
    REASON_INVALID_CLAIM,
    REASON_MALFORMED,
    AppTokenVerifier,
    TokenRejectedError,
)
from thomas_ragx.auth.caller import SCOPE_KB_READ, SCOPE_KB_WRITE, SCOPE_ORG_PROVISION, Caller
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.http_response import request_id_of
from thomas_ragx.platform.request_state import STATE_JTI, STATE_ORG_ID

AUTHORIZATION_HEADER: Final = "authorization"
BEARER_PREFIX: Final = "bearer "
log = structlog.get_logger(__name__)


def _bearer_token(value: str | None) -> str:
    if value is None or not value.lower().startswith(BEARER_PREFIX):
        raise TokenRejectedError(REASON_MALFORMED)
    return value[len(BEARER_PREFIX) :].strip()


def _require_matching_request_id(caller: Caller, request: Request) -> None:
    if caller.request_id != request_id_of(request):
        raise TokenRejectedError(REASON_INVALID_CLAIM)


def require_scope(scope: str) -> Callable[[Request], Awaitable[Caller]]:
    async def dependency(request: Request) -> Caller:
        verifier: AppTokenVerifier = request.app.state.app_token_verifier
        try:
            caller = verifier.verify(_bearer_token(request.headers.get(AUTHORIZATION_HEADER)))
            _require_matching_request_id(caller, request)
        except TokenRejectedError as rejected:
            log.warning("app token rejected", reason=rejected.reason)
            raise AppError(ErrorCode.APP_TOKEN_INVALID) from None
        setattr(request.state, STATE_ORG_ID, caller.org_id)
        setattr(request.state, STATE_JTI, caller.jti)
        structlog.contextvars.bind_contextvars(org_id=caller.org_id)
        if caller.scope != scope:
            log.warning("app token scope denied", required_scope=scope, token_scope=caller.scope)
            raise AppError(ErrorCode.FORBIDDEN)
        return caller

    return dependency


ProvisionCaller = Annotated[Caller, Depends(require_scope(SCOPE_ORG_PROVISION))]
KbReadCaller = Annotated[Caller, Depends(require_scope(SCOPE_KB_READ))]
KbWriteCaller = Annotated[Caller, Depends(require_scope(SCOPE_KB_WRITE))]
