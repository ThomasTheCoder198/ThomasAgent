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
# (field, reason) pairs a failed action may carry to the page; anything else shows the code's catalog message.
# Reasons are the API's `details.fields[].code`; test_ui_messages.py checks every pair has a message.
FIELD_ERRORS: Final = frozenset(
    {
        ("apiKey", "REQUIRED"),
        ("baseUrl", "REQUIRED"),
        ("baseUrl", "INVALID"),
        ("name", "INVALID"),
        ("kind", "INVALID"),
        ("capabilities", "REQUIRED"),
        ("capabilities", "INVALID"),
        ("provider_id", "INVALID"),
        ("model_id", "INVALID"),
        ("role", "INVALID"),
    }
)
FIELD_ERROR_KEY_PREFIX: Final = "field_error"


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


def field_error_key(field: str, reason: str) -> str:
    # Lowercase keys keep the template-key check in test_ui_messages.py simple.
    return f"{FIELD_ERROR_KEY_PREFIX}.{field.lower()}.{reason.lower()}"


def error_message(code: str, field_error: tuple[str, str] | None, lang: Lang) -> str | None:
    """The field-specific text for a validation failure, else the catalog message for the code."""
    if code == ErrorCode.VALIDATION_FAILED.value and field_error is not None and field_error in FIELD_ERRORS:
        return translator(lang)(field_error_key(*field_error))
    return error_text(code, lang)
