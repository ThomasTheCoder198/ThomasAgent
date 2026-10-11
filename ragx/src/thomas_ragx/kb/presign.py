"""Browser-facing MinIO URLs (spec ragx-custom-app §14.3): ragx signs, the browser talks to storage."""

from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Final
from urllib.parse import quote, urlsplit

from minio import Minio
from minio.datatypes import PostPolicy

from thomas_ragx.platform.config import Config

KEY_FIELD: Final = "key"
HTTPS_SCHEME: Final = "https"
METHOD_GET: Final = "GET"
RESPONSE_CONTENT_DISPOSITION: Final = "response-content-disposition"
RESPONSE_CONTENT_TYPE: Final = "response-content-type"
OCTET_STREAM: Final = "application/octet-stream"


@dataclass(frozen=True)
class PresignedPost:
    url: str
    fields: dict[str, str]
    expires_at: datetime


def attachment_disposition(name: str) -> str:
    # RFC 5987 keeps Vietnamese names intact.
    return f"attachment; filename*=UTF-8''{quote(name, safe='')}"


class UrlSigner:
    """Signs for the public endpoint. Its client has an explicit region, so signing never calls the network:
    minio would otherwise ask the public endpoint for the bucket region, which the container cannot reach."""

    def __init__(self, client: Minio, public_url: str) -> None:
        self._client = client
        self._public_url = public_url

    def post_policy(self, bucket: str, key: str, size_bytes: int, expires_at: datetime) -> PresignedPost:
        policy = PostPolicy(bucket, expires_at)
        policy.add_equals_condition(KEY_FIELD, key)
        # Exactly the declared size: storage refuses anything else, so a lying client cannot store more.
        policy.add_content_length_range_condition(size_bytes, size_bytes)
        # presigned_post_policy returns only the signature fields; the key and the URL are ours to add.
        fields = {KEY_FIELD: key, **self._client.presigned_post_policy(policy)}
        return PresignedPost(url=f"{self._public_url}/{bucket}", fields=fields, expires_at=expires_at)

    def download_url(
        self, bucket: str, key: str, file_name: str, expires: timedelta, issued_at: datetime
    ) -> str:
        # attachment + octet-stream: an uploaded .html never renders, and if anything it is MinIO's origin.
        return self._client.get_presigned_url(
            METHOD_GET,
            bucket,
            key,
            expires=expires,
            response_headers={
                RESPONSE_CONTENT_DISPOSITION: attachment_disposition(file_name),
                RESPONSE_CONTENT_TYPE: OCTET_STREAM,
            },
            request_date=issued_at,
        )


def create_url_signer(config: Config) -> UrlSigner:
    parts = urlsplit(config.s3_public_endpoint)
    client = Minio(
        parts.netloc,
        access_key=config.minio_access_key,
        secret_key=config.minio_secret_key.get_secret_value(),
        secure=parts.scheme == HTTPS_SCHEME,
        region=config.s3_region,
    )
    return UrlSigner(client, f"{parts.scheme}://{parts.netloc}")
