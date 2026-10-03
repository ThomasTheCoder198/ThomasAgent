import pytest

from thomas_ragx.platform.config import Config


@pytest.mark.parametrize(
    ("environment_variable", "field_name", "environment_value", "expected", "override", "default"),
    [
        ("RAG_ENV", "environment", "alias-test", "alias-test", "constructor-test", "dev"),
        ("RAG_RETRY_BASE_DELAY_S", "retry_base_delay_seconds", "0.75", 0.75, 0.5, 0.2),
        ("RAG_RETRY_MAX_DELAY_S", "retry_max_delay_seconds", "12.5", 12.5, 8.0, 10.0),
    ],
)
def test_config_preserves_environment_alias_and_named_construction(
    monkeypatch: pytest.MonkeyPatch,
    environment_variable: str,
    field_name: str,
    environment_value: str,
    expected: str | float,
    override: str | float,
    default: str | float,
) -> None:
    monkeypatch.setenv(environment_variable, environment_value)

    config = Config(core_service_token="t", minio_access_key="a", minio_secret_key="s")
    assert getattr(config, field_name) == expected

    configured_by_name = Config(
        **{field_name: override}, core_service_token="t", minio_access_key="a", minio_secret_key="s"
    )
    assert getattr(configured_by_name, field_name) == override

    monkeypatch.delenv(environment_variable)
    monkeypatch.setenv(f"RAG_{field_name.upper()}", environment_value)
    config_without_legacy_variable = Config(
        core_service_token="t", minio_access_key="a", minio_secret_key="s"
    )
    assert getattr(config_without_legacy_variable, field_name) == default


def test_config_preserves_renamed_field_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    for environment_variable in (
        "RAG_ENV",
        "RAG_ENVIRONMENT",
        "RAG_RETRY_BASE_DELAY_S",
        "RAG_RETRY_BASE_DELAY_SECONDS",
        "RAG_RETRY_MAX_DELAY_S",
        "RAG_RETRY_MAX_DELAY_SECONDS",
    ):
        monkeypatch.delenv(environment_variable, raising=False)

    config = Config(core_service_token="t", minio_access_key="a", minio_secret_key="s")

    assert config.environment == "dev"
    assert config.retry_base_delay_seconds == 0.2
    assert config.retry_max_delay_seconds == 10.0
