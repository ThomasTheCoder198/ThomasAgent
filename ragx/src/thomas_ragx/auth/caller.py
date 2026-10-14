import re
from dataclasses import dataclass
from typing import Final

SCOPE_ORG_PROVISION: Final = "org:provision"
SCOPE_KB_READ: Final = "kb:read"
SCOPE_KB_WRITE: Final = "kb:write"
TOOL_SCOPE_PREFIX: Final = "tool:"
TOOL_KB_SEARCH: Final = "kb_search"
TOOL_KB_OUTLINE: Final = "kb_outline"
TOOL_KB_READ_SECTION: Final = "kb_read_section"
TOOL_KB_LIST_DOCUMENTS: Final = "kb_list_documents"
ORG_ID_PATTERN: Final = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


@dataclass(frozen=True)
class Caller:
    """Who core says is calling: the org comes only from the signed token, never from the request body."""

    org_id: str
    scope: str
    kb_id: str | None
    request_id: str
    jti: str
