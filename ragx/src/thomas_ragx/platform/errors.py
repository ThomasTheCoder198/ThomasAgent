from enum import StrEnum
from typing import Any

from thomas_ragx.platform.error_codes_gen import ERROR_DEFINITIONS, ErrorCode


class Lang(StrEnum):
    VI = "vi"
    EN = "en"


def lang_from_header(value: str | None) -> Lang:
    return Lang.EN if (value or "").strip().lower().startswith(Lang.EN) else Lang.VI


class AppError(Exception):
    def __init__(
        self, code: ErrorCode, message: str | None = None, details: dict[str, Any] | None = None
    ) -> None:
        super().__init__(code.value)
        self.code = code
        self.message = message
        self.details = details

    def http_status(self) -> int:
        return ERROR_DEFINITIONS[self.code].http_status

    def retryable(self) -> bool:
        return ERROR_DEFINITIONS[self.code].retryable

    def localized_message(self, lang: Lang) -> str:
        if self.message:
            return self.message
        spec = ERROR_DEFINITIONS[self.code]
        return spec.message_en if lang is Lang.EN else spec.message_vi
