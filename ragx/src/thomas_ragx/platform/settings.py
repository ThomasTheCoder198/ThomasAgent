from functools import lru_cache
from typing import Literal

from pydantic import SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="RAG_", env_file=None)

    env: str = "dev"
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
    retry_base_delay_s: float = 0.2
    retry_max_delay_s: float = 10.0


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()  # type: ignore[call-arg]  # Required values are loaded from the environment.
