import base64
import json
import time
import uuid
from pathlib import Path
from typing import Any

import jwt
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

VECTOR: dict[str, Any] = json.loads(
    (Path(__file__).resolve().parents[2] / "contracts" / "app-token.json").read_text(encoding="utf-8")
)
TRUSTED_KID: str = VECTOR["keys"][0]["kid"]
_PRIVATE_KEY = Ed25519PrivateKey.from_private_bytes(base64.b64decode(VECTOR["keys"][0]["privateKeySeed"]))
TOKEN_LIFETIME_SECONDS = 50


def public_keys_config() -> dict[str, str]:
    return {key["kid"]: key["publicKey"] for key in VECTOR["keys"]}


def mint(org_id: str, scope: str, kb_id: str | None = None, **overrides: object) -> str:
    """Test-only stand-in for core's signer (M1.2): same key as the vector, current time."""
    now = int(time.time())
    claims: dict[str, object] = {
        "iss": VECTOR["issuer"],
        "aud": VECTOR["audience"],
        "oid": org_id,
        "scp": scope,
        "rid": f"req_{uuid.uuid4()}",
        "jti": str(uuid.uuid4()),
        "iat": now,
        "exp": now + TOKEN_LIFETIME_SECONDS,
    }
    if kb_id is not None:
        claims["kb"] = kb_id
    claims.update(overrides)
    return jwt.encode(
        claims, _PRIVATE_KEY, algorithm="EdDSA", headers={"kid": TRUSTED_KID, "typ": VECTOR["type"]}
    )
