import httpx
import litellm
import pytest

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.providers.litellm_errors import to_app_error

REQUEST = httpx.Request("POST", "https://p.example")


@pytest.mark.parametrize(
    ("exc", "code"),
    [
        (litellm.RateLimitError("slow down", "openai", "m"), ErrorCode.PROVIDER_UNAVAILABLE),
        (litellm.Timeout("t", "m", "openai"), ErrorCode.PROVIDER_UNAVAILABLE),
        (litellm.APIConnectionError("down", "openai", "m"), ErrorCode.PROVIDER_UNAVAILABLE),
        (litellm.ServiceUnavailableError("503", "openai", "m"), ErrorCode.PROVIDER_UNAVAILABLE),
        (litellm.AuthenticationError("bad key", "openai", "m"), ErrorCode.LLM_PROVIDER_REJECTED),
        (litellm.BadRequestError("bad", "m", "openai"), ErrorCode.LLM_PROVIDER_REJECTED),
        (ValueError("other"), ErrorCode.PROVIDER_UNAVAILABLE),
    ],
)
def test_litellm_errors_map_to_catalog_codes(exc: Exception, code: ErrorCode) -> None:
    assert to_app_error(exc).code is code
