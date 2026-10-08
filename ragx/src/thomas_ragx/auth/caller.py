import re
from dataclasses import dataclass
from typing import Final

SCOPE_ORG_PROVISION: Final = "org:provision"
SCOPE_KB_READ: Final = "kb:read"
SCOPE_KB_WRITE: Final = "kb:write"
TOOL_SCOPE_PREFIX: Final = "tool:"
ORG_ID_PATTERN: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


@dataclass(frozen=True)
class Caller:
    """Who core says is calling: the org comes only from the signed token, never from the request body."""

    org_id: str
    scope: str
    kb_id: str | None
    request_id: str
    jti: str
