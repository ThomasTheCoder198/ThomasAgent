import json
from collections.abc import Callable
from functools import cache
from importlib.resources import files
from typing import Final

from starlette.requests import Request

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError, Lang, lang_from_header

LANG_COOKIE: Final = "ragx_lang"
LANG_VALUES: Final = frozenset(lang.value for lang in Lang)


@cache
def messages(lang: Lang) -> dict[str, str]:
    raw = files("thomas_ragx.ui").joinpath("messages", f"{lang.value}.json").read_text(encoding="utf-8")
    return dict(json.loads(raw))


def ui_language(request: Request) -> Lang:
    chosen = request.cookies.get(LANG_COOKIE)
    if chosen in LANG_VALUES:
        return Lang(chosen)
    return lang_from_header(request.headers.get("accept-language"))


def translator(lang: Lang) -> Callable[[str], str]:
    table = messages(lang)
    return lambda key: table.get(key, key)


def error_text(code: str, lang: Lang) -> str | None:
    try:
        return AppError(ErrorCode(code)).localized_message(lang)
    except ValueError:
        return None
