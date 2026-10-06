import re
import uuid
from typing import Final, TypeGuard

# Values mirror contracts/request-id.json; tests/unit/test_request_id.py fails when they drift.
REQUEST_ID_HEADER: Final = "X-Request-Id"
REQUEST_ID_PATTERN: Final = r"^(req|job)_[A-Za-z0-9-]{8,64}$"
REQUEST_PREFIX: Final = "req_"
JOB_PREFIX: Final = "job_"
REQUEST_ID_SOURCE_GENERATED: Final = "generated"
_VALID_ID: Final = re.compile(REQUEST_ID_PATTERN)


def is_valid_request_id(value: str | None) -> TypeGuard[str]:
    # fullmatch: with re.match, "$" would also accept a trailing newline.
    return value is not None and _VALID_ID.fullmatch(value) is not None


def new_request_id() -> str:
    return f"{REQUEST_PREFIX}{uuid.uuid4()}"


def accept_request_id(sent: str | None) -> tuple[str, bool]:
    """Return the id to use and whether a client-sent value was rejected (it is never logged)."""
    if is_valid_request_id(sent):
        return sent, False
    return new_request_id(), bool(sent)
