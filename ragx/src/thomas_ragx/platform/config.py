from functools import lru_cache
from typing import Literal

from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, PydanticBaseSettingsSource, SettingsConfigDict


class Config(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="RAG_", env_file=None, validate_by_name=True)

    environment: str = Field(default="dev", validation_alias="RAG_ENV")
    service_name: str = "thomas-ragx"
    http_host: str = "0.0.0.0"  # noqa: S104 - container listens on all interfaces by design
    http_port: int = 8090
    log_level: Literal["debug", "info", "warn", "error"] = "info"
    otlp_endpoint: str = ""
    core_internal_url: str = "http://core:8080"
    core_service_token: SecretStr
    qdrant_url: str = "http://qdrant:6333"
    minio_endpoint: str = "minio:9000"
    minio_access_key: str
    minio_secret_key: SecretStr
    minio_secure: bool = False
    buckets: tuple[str, ...] = ("kb-raw", "kb-preview", "chat-attachments")
    retry_max_attempts: int = 4
    retry_base_delay_seconds: float = Field(default=0.2, validation_alias="RAG_RETRY_BASE_DELAY_S")
    retry_max_delay_seconds: float = Field(default=10.0, validation_alias="RAG_RETRY_MAX_DELAY_S")

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
        return init_settings, env_settings, dotenv_settings, file_secret_settings


@lru_cache(maxsize=1)
def load_config() -> Config:
    return Config()  # type: ignore[call-arg]  # Required values are loaded from the environment.
