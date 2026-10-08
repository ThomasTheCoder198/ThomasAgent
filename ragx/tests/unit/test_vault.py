import base64
import json
import uuid

import pytest
import structlog
from pydantic import SecretStr

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.registry.vault import Vault, provider_key_aad

KEY_A = SecretStr(base64.b64encode(b"a" * 32).decode())
KEY_B = SecretStr(base64.b64encode(b"b" * 32).decode())
API_KEY = "sk-or-v1-0123456789abcdef"


def test_seal_and_open_round_trip_with_the_same_aad() -> None:
    aad = provider_key_aad(uuid.uuid4())
    sealed = Vault(KEY_A).seal(API_KEY, aad)
    assert API_KEY.encode() not in sealed.ciphertext
    assert Vault(KEY_A).open(sealed, aad).get_secret_value() == API_KEY


def test_vault_wrong_master_key_fails_closed_without_leaking(capsys: pytest.CaptureFixture[str]) -> None:
    configure_logging("info", "thomas-ragx", "test")
    structlog.contextvars.clear_contextvars()
    aad = provider_key_aad(uuid.uuid4())
    sealed = Vault(KEY_A).seal(API_KEY, aad)
    with pytest.raises(AppError) as failure:
        Vault(KEY_B).open(sealed, aad)
    assert failure.value.code is ErrorCode.INTERNAL_ERROR
    output = capsys.readouterr().out
    records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
    assert any(record["msg"] == "vault decrypt failed" for record in records)
    for secret in (
        API_KEY,
        KEY_A.get_secret_value(),
        KEY_B.get_secret_value(),
        base64.b64encode(sealed.nonce).decode(),
    ):
        assert secret not in output


def test_ciphertext_is_bound_to_its_provider() -> None:
    sealed = Vault(KEY_A).seal(API_KEY, provider_key_aad(uuid.uuid4()))
    with pytest.raises(AppError):
        Vault(KEY_A).open(sealed, provider_key_aad(uuid.uuid4()))


@pytest.mark.parametrize("value", ["not base64!", base64.b64encode(b"short").decode()])
def test_master_key_must_be_32_bytes_of_base64(value: str) -> None:
    with pytest.raises(ValueError, match="RAG_VAULT_MASTER_KEY"):
        Vault(SecretStr(value))
