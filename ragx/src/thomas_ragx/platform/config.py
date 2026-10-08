import os
from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr
from pydantic_settings import (
    BaseSettings,
    PydanticBaseSettingsSource,
    SettingsConfigDict,
    YamlConfigSettingsSource,
)

DEFAULT_CONFIG_FILE = Path(__file__).with_name("config.yaml")


class Config(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="RAG_", env_file=".env", validate_by_name=True, extra="ignore"
    )

    environment: str = Field(validation_alias="RAG_ENV")
    service_name: str
    http_host: str
    http_port: int
    log_level: Literal["debug", "info", "warn", "error"]
    otlp_endpoint: str
    phoenix_api_key: SecretStr | None = None
    phoenix_project: str
    trace_content: bool
    database_url: SecretStr
    database_pool_min_size: int
    database_pool_max_size: int
    database_connect_timeout_seconds: float
    readiness_timeout_seconds: float
    redis_url: str
    vault_master_key: SecretStr
    core_public_keys: dict[str, str]
    app_token_issuer: str
    app_token_audience: str
    app_token_leeway_seconds: int
    app_token_max_lifetime_seconds: int
    qdrant_url: str
    minio_endpoint: str
    minio_access_key: str
    minio_secret_key: SecretStr
    minio_secure: bool
    buckets: tuple[str, ...]
    upload_max_bytes: int
    upload_multipart_overhead_bytes: int
    idempotency_ttl_seconds: int
    page_default_limit: int
    page_max_limit: int
    retry_max_attempts: int
    outbox_batch_size: int
    outbox_poll_interval_seconds: float
    stream_batch_size: int
    stream_block_milliseconds: int
    stream_visibility_timeout_milliseconds: int
    stream_max_deliveries: int
    worker_heartbeat_file: str
    worker_metrics_export_interval_milliseconds: int
    retry_base_delay_seconds: float = Field(validation_alias="RAG_RETRY_BASE_DELAY_S")
    retry_max_delay_seconds: float = Field(validation_alias="RAG_RETRY_MAX_DELAY_S")

    @classmethod
    def settings_customise_sources(
        cls,
        settings_cls: type[BaseSettings],
        init_settings: PydanticBaseSettingsSource,
        env_settings: PydanticBaseSettingsSource,
        dotenv_settings: PydanticBaseSettingsSource,
        file_secret_settings: PydanticBaseSettingsSource,
    ) -> tuple[PydanticBaseSettingsSource, ...]:
        # Constructor names must not introduce new environment variable spellings.
        env_settings.config = {
            **env_settings.config,
            "validate_by_name": False,
            "populate_by_name": False,
        }
        dotenv_settings.config = {
            **dotenv_settings.config,
            "validate_by_name": False,
            "populate_by_name": False,
        }
        return (
            init_settings,
            env_settings,
            dotenv_settings,
            YamlConfigSettingsSource(settings_cls, yaml_file=DEFAULT_CONFIG_FILE),
            file_secret_settings,
        )


@lru_cache(maxsize=1)
def load_config() -> Config:
    env_file = Path(os.environ.get("RAG_ENV_FILE", ".env"))
    return Config(_env_file=env_file)  # type: ignore[call-arg]  # Required secrets come from dotenv or environment.
