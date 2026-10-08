from typing import Any

import pytest

from tests.tokens import VECTOR, public_keys_config
from thomas_ragx.auth.app_token import AppTokenVerifier, TokenRejectedError, load_public_keys


def _verifier() -> AppTokenVerifier:
    return AppTokenVerifier(
        load_public_keys(public_keys_config()),
        issuer=VECTOR["issuer"],
        audience=VECTOR["audience"],
        leeway_seconds=VECTOR["leewaySeconds"],
        max_lifetime_seconds=VECTOR["maxLifetimeSeconds"],
        clock=lambda: float(VECTOR["now"]),
    )


@pytest.mark.parametrize("case", VECTOR["valid"], ids=lambda case: case["name"])
def test_verifier_accepts_every_valid_vector_token(case: dict[str, Any]) -> None:
    caller = _verifier().verify(case["token"])
    assert caller.org_id == case["claims"]["oid"]
    assert caller.scope == case["claims"]["scp"]
    assert caller.kb_id == case["claims"].get("kb")
    assert caller.request_id == case["claims"]["rid"]
    assert caller.jti == case["claims"]["jti"]


@pytest.mark.parametrize("case", VECTOR["invalid"], ids=lambda case: case["name"])
def test_verifier_rejects_every_invalid_vector_token_with_its_reason(case: dict[str, Any]) -> None:
    with pytest.raises(TokenRejectedError) as rejected:
        _verifier().verify(case["token"])
    assert rejected.value.reason == case["reason"]


def test_load_public_keys_refuses_a_key_that_is_not_32_raw_bytes() -> None:
    with pytest.raises(ValueError, match="dev-1"):
        load_public_keys({"dev-1": "c2hvcnQ="})
