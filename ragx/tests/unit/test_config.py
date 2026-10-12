from datetime import timedelta
from pathlib import Path

import pytest
from pydantic import ValidationError

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

    config = Config(minio_access_key="a", minio_secret_key="s")
    assert getattr(config, field_name) == expected

    configured_by_name = Config(**{field_name: override}, minio_access_key="a", minio_secret_key="s")
    assert getattr(configured_by_name, field_name) == override

    monkeypatch.delenv(environment_variable)
    monkeypatch.setenv(f"RAG_{field_name.upper()}", environment_value)
    config_without_legacy_variable = Config(minio_access_key="a", minio_secret_key="s")
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

    config = Config(minio_access_key="a", minio_secret_key="s")

    assert config.environment == "dev"
    assert config.retry_base_delay_seconds == 0.2
    assert config.retry_max_delay_seconds == 10.0


def test_config_layers_yaml_dotenv_environment_and_constructor(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text(
        "RAG_VAULT_MASTER_KEY=local-vault-key\n"
        "RAG_MINIO_ACCESS_KEY=local-access\n"
        "RAG_MINIO_SECRET_KEY=local-secret\n"
        "RAG_RETRY_MAX_ATTEMPTS=7\n"
        "POSTGRES_USER=unrelated-value\n",
        encoding="utf-8",
    )
    monkeypatch.delenv("RAG_RETRY_MAX_ATTEMPTS", raising=False)
    monkeypatch.delenv("RAG_VAULT_MASTER_KEY", raising=False)
    config = Config(_env_file=env_file)
    assert config.retry_max_attempts == 7
    assert config.http_port == 8090
    assert config.vault_master_key.get_secret_value() == "local-vault-key"

    monkeypatch.setenv("RAG_RETRY_MAX_ATTEMPTS", "3")
    assert Config(_env_file=env_file).retry_max_attempts == 3
    assert Config(_env_file=env_file, retry_max_attempts=2).retry_max_attempts == 2


def test_presign_settings_have_safe_defaults_and_are_validated(monkeypatch: pytest.MonkeyPatch) -> None:
    config = Config(minio_access_key="a", minio_secret_key="s")
    assert config.s3_public_endpoint == "http://localhost:9000"
    assert config.s3_region == "us-east-1"
    assert config.upload_url_ttl == timedelta(minutes=15)
    assert config.download_url_ttl == timedelta(seconds=60)
    monkeypatch.setenv("RAG_DOWNLOAD_URL_TTL", "PT30S")  # env overrides are ISO 8601
    assert Config(minio_access_key="a", minio_secret_key="s").download_url_ttl == timedelta(seconds=30)
    for field, value in [
        ("s3_public_endpoint", "localhost:9000"),
        ("s3_public_endpoint", "ftp://files.example"),
        ("s3_public_endpoint", "http://files.example/prefix"),
        ("upload_url_ttl", timedelta(0)),
        ("download_url_ttl", timedelta(days=8)),
    ]:
        with pytest.raises(ValidationError):
            Config(minio_access_key="a", minio_secret_key="s", **{field: value})


def test_config_defaults_for_the_rag_pipeline() -> None:
    config = Config(minio_access_key="a", minio_secret_key="s")
    assert config.upload_max_bytes == 52_428_800
    assert config.ingest_concurrency == 2
    assert config.ingest_visibility_timeout_milliseconds == 1_800_000
    assert config.worker_heartbeat_interval_seconds == 10.0
    assert config.sweep_interval_seconds == 600.0
    assert config.upload_sweep_grace_seconds == 300
    assert config.datalab_api_key is None
    assert config.ocr_base_url == "https://www.datalab.to"
    assert (config.datalab_mode, config.datalab_retry_mode) == ("balanced", "accurate")
    assert config.datalab_min_quality == 3.0
    assert config.datalab_skip_cache is True


def test_config_rejects_a_visibility_timeout_shorter_than_two_converts() -> None:
    with pytest.raises(ValidationError):
        Config(
            minio_access_key="a",
            minio_secret_key="s",
            datalab_timeout_seconds=600.0,
            ingest_visibility_timeout_milliseconds=1_200_000,
        )


def test_config_rejects_zero_ingest_concurrency() -> None:
    with pytest.raises(ValidationError):
        Config(minio_access_key="a", minio_secret_key="s", ingest_concurrency=0)


def test_datalab_key_comes_only_from_the_rag_variable(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATALAB_API_KEY", "ignored")
    assert Config(minio_access_key="a", minio_secret_key="s").datalab_api_key is None
    monkeypatch.setenv("RAG_DATALAB_API_KEY", "dl-key")
    key = Config(minio_access_key="a", minio_secret_key="s").datalab_api_key
    assert key is not None and key.get_secret_value() == "dl-key"
