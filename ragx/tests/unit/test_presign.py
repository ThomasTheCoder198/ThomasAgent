import base64
import json
from datetime import UTC, datetime, timedelta
from urllib.parse import parse_qs, urlsplit

from minio import Minio

from thomas_ragx.kb.presign import UrlSigner, create_url_signer
from thomas_ragx.platform.config import Config

HOST = "storage.example.invalid:9000"


def _signer() -> UrlSigner:
    client = Minio(HOST, access_key="ak", secret_key="sk-long-enough", secure=False, region="us-east-1")
    return UrlSigner(client, f"http://{HOST}")


def test_post_policy_pins_the_key_and_the_exact_size_without_network() -> None:
    post = _signer().post_policy("kb-raw", "org-a/kb/file", 10, datetime(2030, 1, 1, tzinfo=UTC))
    assert post.url == f"http://{HOST}/kb-raw"
    assert post.fields["key"] == "org-a/kb/file"
    assert {
        "policy",
        "x-amz-algorithm",
        "x-amz-credential",
        "x-amz-date",
        "x-amz-signature",
    } <= post.fields.keys()
    policy = json.loads(base64.b64decode(post.fields["policy"]))
    assert policy["expiration"].startswith("2030-01-01T00:00:00")
    assert ["content-length-range", 10, 10] in policy["conditions"]
    assert ["eq", "$key", "org-a/kb/file"] in policy["conditions"]
    assert ["eq", "$bucket", "kb-raw"] in policy["conditions"]


def test_download_url_forces_attachment_and_octet_stream() -> None:
    issued = datetime(2030, 1, 1, tzinfo=UTC)
    url = _signer().download_url("kb-raw", "org-a/kb/file", "Tài liệu.html", timedelta(seconds=60), issued)
    parts = urlsplit(url)
    query = parse_qs(parts.query)
    assert (parts.scheme, parts.netloc, parts.path) == ("http", HOST, "/kb-raw/org-a/kb/file")
    assert query["response-content-type"] == ["application/octet-stream"]
    assert query["response-content-disposition"] == [
        "attachment; filename*=UTF-8''T%C3%A0i%20li%E1%BB%87u.html"
    ]
    assert query["X-Amz-Expires"] == ["60"]
    assert query["X-Amz-Date"] == ["20300101T000000Z"]


def test_create_url_signer_takes_scheme_and_host_from_the_public_endpoint() -> None:
    config = Config(
        minio_access_key="a", minio_secret_key="s", s3_public_endpoint="https://files.example.invalid"
    )
    post = create_url_signer(config).post_policy("kb-raw", "k", 1, datetime(2030, 1, 1, tzinfo=UTC))
    assert post.url == "https://files.example.invalid/kb-raw"
