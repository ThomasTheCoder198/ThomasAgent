import json
from pathlib import Path
from typing import Any

from thomas_ragx.platform import request_id

CONTRACT = Path(__file__).resolve().parents[3] / "contracts" / "request-id.json"


def _contract() -> dict[str, Any]:
    data: dict[str, Any] = json.loads(CONTRACT.read_text(encoding="utf-8"))
    return data


def test_request_id_matches_shared_contract() -> None:
    contract = _contract()
    assert contract["header"] == request_id.REQUEST_ID_HEADER
    assert contract["pattern"] == request_id.REQUEST_ID_PATTERN
    assert contract["requestPrefix"] == request_id.REQUEST_PREFIX
    assert contract["jobPrefix"] == request_id.JOB_PREFIX


def test_is_valid_request_id_follows_shared_examples() -> None:
    contract = _contract()
    for value in contract["valid"]:
        assert request_id.is_valid_request_id(value), value
    for value in contract["invalid"]:
        assert not request_id.is_valid_request_id(value), value


def test_request_id_rejects_trailing_newline() -> None:
    assert not request_id.is_valid_request_id("req_abcdefgh\n")
    assert not request_id.is_valid_request_id(None)


def test_accept_request_id_replaces_invalid_values() -> None:
    sent = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
    assert request_id.accept_request_id(sent) == (sent, False)
    generated, rejected = request_id.accept_request_id("forged id")
    assert rejected is True
    assert generated.startswith(request_id.REQUEST_PREFIX)
    assert request_id.is_valid_request_id(generated)
    missing, rejected = request_id.accept_request_id(None)
    assert rejected is False
    assert request_id.is_valid_request_id(missing)
