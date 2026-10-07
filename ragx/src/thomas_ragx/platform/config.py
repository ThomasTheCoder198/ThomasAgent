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
    core_internal_url: str
    core_service_token: SecretStr
    qdrant_url: str
    minio_endpoint: str
    minio_access_key: str
    minio_secret_key: SecretStr
    minio_secure: bool
    buckets: tuple[str, ...]
    retry_max_attempts: int
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
