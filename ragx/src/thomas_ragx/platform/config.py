import os
from datetime import timedelta
from functools import lru_cache
from pathlib import Path
from typing import Final, Literal
from urllib.parse import urlsplit

from pydantic import Field, SecretStr, field_validator, model_validator
from pydantic_settings import (
    BaseSettings,
    PydanticBaseSettingsSource,
    SettingsConfigDict,
    YamlConfigSettingsSource,
)

DEFAULT_CONFIG_FILE = Path(__file__).with_name("config.yaml")
PUBLIC_ENDPOINT_SCHEMES: Final = frozenset({"http", "https"})
PRESIGN_MIN_TTL: Final = timedelta(seconds=1)
PRESIGN_MAX_TTL: Final = timedelta(days=7)
MILLISECONDS_PER_SECOND: Final = 1000
CONVERTS_PER_FILE: Final = 2  # balanced, then accurate when quality is low (ruling A7)


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
    s3_public_endpoint: str
    s3_region: str
    upload_url_ttl: timedelta
    download_url_ttl: timedelta
    buckets: tuple[str, ...]
    upload_max_bytes: int
    ingest_concurrency: int
    ingest_visibility_timeout_milliseconds: int
    worker_heartbeat_interval_seconds: float
    sweep_interval_seconds: float
    upload_sweep_grace_seconds: int
    work_dir: str
    datalab_api_key: SecretStr | None = None
    ocr_base_url: str
    datalab_mode: str
    datalab_retry_mode: str
    datalab_min_quality: float
    datalab_timeout_seconds: float
    datalab_poll_interval_seconds: int
    datalab_skip_cache: bool
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
    admin_email: str
    admin_initial_password: SecretStr | None = None
    ui_session_ttl_seconds: int
    ui_cookie_secure: bool
    ui_login_max_attempts: int
    ui_login_window_seconds: int
    retry_base_delay_seconds: float = Field(validation_alias="RAG_RETRY_BASE_DELAY_S")
    retry_max_delay_seconds: float = Field(validation_alias="RAG_RETRY_MAX_DELAY_S")

    @field_validator("s3_public_endpoint")
    @classmethod
    def _public_endpoint_is_an_origin(cls, value: str) -> str:
        """The browser POSTs and GETs here; ragx signs for it but never connects to it."""
        parts = urlsplit(value)
        if (
            parts.scheme not in PUBLIC_ENDPOINT_SCHEMES
            or not parts.netloc
            or parts.path not in ("", "/")
            or parts.query
        ):
            raise ValueError(
                "RAG_S3_PUBLIC_ENDPOINT must be an http(s) origin such as https://files.example.com"
            )
        return value.rstrip("/")

    @field_validator("upload_url_ttl", "download_url_ttl")
    @classmethod
    def _presign_ttl_in_range(cls, value: timedelta) -> timedelta:
        if not PRESIGN_MIN_TTL <= value <= PRESIGN_MAX_TTL:  # MinIO accepts 1 second to 7 days
            raise ValueError("presigned URL lifetimes must be between 1 second and 7 days")
        return value

    @field_validator("ingest_concurrency")
    @classmethod
    def _at_least_one_ingest_consumer(cls, value: int) -> int:
        if value < 1:
            raise ValueError("RAG_INGEST_CONCURRENCY must be at least 1")
        return value

    @model_validator(mode="after")
    def _ingest_lease_outlives_a_job(self) -> "Config":
        # A reclaim while Datalab still works would run the file twice (ruling A11).
        bounded_job_ms = CONVERTS_PER_FILE * self.datalab_timeout_seconds * MILLISECONDS_PER_SECOND
        margin_ms = self.stream_visibility_timeout_milliseconds
        if self.ingest_visibility_timeout_milliseconds <= bounded_job_ms + margin_ms:
            raise ValueError(
                "RAG_INGEST_VISIBILITY_TIMEOUT_MILLISECONDS must exceed two Datalab timeouts plus "
                "RAG_STREAM_VISIBILITY_TIMEOUT_MILLISECONDS"
            )
        return self

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
