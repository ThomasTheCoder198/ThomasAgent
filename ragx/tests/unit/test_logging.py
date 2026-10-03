from thomas_ragx.platform.logging import REDACTED, redact


def test_redacts_secret_keys_and_values() -> None:
    event = {
        "event": "calling provider",
        "api_key": "sk-or-v1-abc",
        "headers": {"Authorization": "Bearer xyz", "accept": "json"},
        "note": "token sk-live-123 leaked",
        "input_tokens": 392,
    }
    out = redact(None, "info", event)
    assert out["input_tokens"] == 392
    assert out["api_key"] == REDACTED
    assert out["headers"]["Authorization"] == REDACTED
    assert out["headers"]["accept"] == "json"
    assert "sk-live-123" not in out["note"]


def test_redacts_nested_list_and_tuple_containers() -> None:
    details = {
        "providers": [{"api_key": "plain-secret", "input_tokens": 392}],
        "nested": ([{"password": "plain-secret"}],),
        "values": ["Bearer plain-secret"],
        "token": [{"value": "plain-secret"}],
    }
    cleaned = redact(None, "info", {"details": details})["details"]
    assert "plain-secret" not in str(cleaned)
    assert cleaned["providers"][0]["api_key"] == REDACTED
    assert cleaned["providers"][0]["input_tokens"] == 392
    assert cleaned["token"] == REDACTED
    assert isinstance(cleaned["nested"], tuple)
    assert details["providers"][0]["api_key"] == "plain-secret"
