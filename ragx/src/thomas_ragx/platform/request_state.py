from typing import Final

# Names on request.state written by the App-token dependency and read by the access-log middleware;
# contextvars cannot carry them because the middleware runs downstream code in a copied context.
STATE_ORG_ID: Final = "org_id"
STATE_JTI: Final = "jti"

STATE_TOOL: Final = "tool"
