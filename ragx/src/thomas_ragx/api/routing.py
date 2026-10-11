from collections.abc import Callable, Coroutine
from typing import Any, Final

from fastapi import APIRouter, Request, Response
from fastapi.routing import APIRoute
from starlette.routing import Match
from starlette.types import Receive, Scope, Send

METHOD_GET: Final = "GET"
METHOD_HEAD: Final = "HEAD"


class HeadCapableRoute(APIRoute):
    """FastAPI, unlike Starlette's plain Route, answers a bare 405 to HEAD on a GET route.

    Core forwards HEAD to every route its proxy reaches, so each GET route serves HEAD with the status and
    headers of GET and no body. HEAD has no body on the wire, even for errors: a failed one has a status only.
    """

    def _serves_as_head(self, scope: Scope) -> bool:
        # self.methods stays GET-only: HEAD is then not a second operation in the OpenAPI schema.
        return scope["method"] == METHOD_HEAD and self.methods is not None and METHOD_GET in self.methods

    def matches(self, scope: Scope) -> tuple[Match, Scope]:
        if scope["type"] == "http" and self._serves_as_head(scope):
            return super().matches({**scope, "method": METHOD_GET})
        return super().matches(scope)

    async def handle(self, scope: Scope, receive: Receive, send: Send) -> None:
        if self._serves_as_head(scope):
            await self.app(scope, receive, send)  # Route.handle would refuse the method
            return
        await super().handle(scope, receive, send)

    def get_route_handler(self) -> Callable[[Request], Coroutine[Any, Any, Response]]:
        handler = super().get_route_handler()

        async def serve(request: Request) -> Response:
            response = await handler(request)
            if request.method == METHOD_HEAD:
                # Content-Length stays that of the GET body (RFC 9110 §9.3.2).
                response.body = b""
            return response

        return serve


def api_router(prefix: str = "") -> APIRouter:
    return APIRouter(prefix=prefix, route_class=HeadCapableRoute)
