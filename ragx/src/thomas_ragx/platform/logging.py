import logging
import re
from collections.abc import MutableMapping
from typing import Any, Final

import structlog
from opentelemetry import trace

REDACTED: Final = "[REDACTED]"
# Whole words preserve input_tokens while hiding csrf_token and X-Api-Key.
SECRET_KEY_WORDS: Final = frozenset(
    {"secret", "password", "passwd", "token", "authorization", "cookie", "apikey"}
)
SECRET_KEY_PHRASES: Final = ("api_key", "private_key")
KEY_WORD_SPLITTER: Final = re.compile(r"[^a-z0-9]+")
SECRET_VALUE_PATTERN: Final = re.compile(r"(?i)(bearer\s+\S+|sk-[a-z0-9_\-]{3,}\S*)")
LEVELS: Final = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warn": logging.WARNING,
    "error": logging.ERROR,
}


def _is_secret_key(key: str) -> bool:
    lowered = key.lower()
    if any(phrase in lowered.replace("-", "_") for phrase in SECRET_KEY_PHRASES):
        return True
    return any(word in SECRET_KEY_WORDS for word in KEY_WORD_SPLITTER.split(lowered))


def _clean(key: str, value: Any) -> Any:
    if _is_secret_key(key):
        return REDACTED
    if isinstance(value, str):
        return SECRET_VALUE_PATTERN.sub(REDACTED, value)
    if isinstance(value, dict):
        return {k: _clean(str(k), v) for k, v in value.items()}
    if isinstance(value, list):
        return [_clean("", item) for item in value]
    if isinstance(value, tuple):
        return tuple(_clean("", item) for item in value)
    return value


def redact(_logger: Any, _method: str, event_dict: MutableMapping[str, Any]) -> MutableMapping[str, Any]:
    for key in list(event_dict):
        event_dict[key] = _clean(key, event_dict[key])
    return event_dict


def _add_trace(_logger: Any, _method: str, event_dict: MutableMapping[str, Any]) -> MutableMapping[str, Any]:
    ctx = trace.get_current_span().get_span_context()
    if ctx.is_valid:
        event_dict["trace_id"] = format(ctx.trace_id, "032x")
        event_dict["span_id"] = format(ctx.span_id, "016x")
    return event_dict


def configure_logging(level: str, service: str, environment: str) -> None:
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso", key="ts"),
            _add_trace,
            redact,
            structlog.processors.EventRenamer("msg"),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.make_filtering_bound_logger(LEVELS[level]),
        cache_logger_on_first_use=True,
    )
    structlog.contextvars.bind_contextvars(service=service, env=environment)
