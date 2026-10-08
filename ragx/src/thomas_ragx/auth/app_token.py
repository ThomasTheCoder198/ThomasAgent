import base64
import binascii
import time
import uuid
from collections.abc import Callable, Mapping
from typing import Any, Final

import jwt
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

from thomas_ragx.auth.caller import ORG_ID_PATTERN, Caller
from thomas_ragx.platform.request_id import is_valid_request_id

ALGORITHM: Final = "EdDSA"
TOKEN_TYPE: Final = "thomas-app+jwt"  # noqa: S105  # JWT header type, not a credential.
CLAIM_ORG: Final = "oid"
CLAIM_KB: Final = "kb"
CLAIM_SCOPE: Final = "scp"
CLAIM_REQUEST_ID: Final = "rid"
CLAIM_JTI: Final = "jti"
CLAIM_ISSUED_AT: Final = "iat"
CLAIM_EXPIRES: Final = "exp"
REQUIRED_REGISTERED_CLAIMS: Final = ("iss", "aud", CLAIM_ISSUED_AT, CLAIM_EXPIRES, CLAIM_JTI)
ED25519_PUBLIC_KEY_BYTES: Final = 32

REASON_MALFORMED: Final = "malformed"
REASON_WRONG_ALGORITHM: Final = "wrong_algorithm"
REASON_WRONG_TYPE: Final = "wrong_type"
REASON_UNKNOWN_KID: Final = "unknown_kid"
REASON_BAD_SIGNATURE: Final = "bad_signature"
REASON_WRONG_ISSUER: Final = "wrong_issuer"
REASON_WRONG_AUDIENCE: Final = "wrong_audience"
REASON_MISSING_CLAIM: Final = "missing_claim"
REASON_INVALID_CLAIM: Final = "invalid_claim"
REASON_EXPIRED: Final = "expired"
REASON_NOT_YET_VALID: Final = "not_yet_valid"
REASON_LIFETIME_TOO_LONG: Final = "lifetime_too_long"


class TokenRejectedError(Exception):
    """Carries a fixed reason code; the token itself is never part of the message."""

    def __init__(self, reason: str) -> None:
        super().__init__(reason)
        self.reason = reason


def load_public_keys(encoded: Mapping[str, str]) -> dict[str, Ed25519PublicKey]:
    keys: dict[str, Ed25519PublicKey] = {}
    for kid, value in encoded.items():
        try:
            raw = base64.b64decode(value, validate=True)
        except binascii.Error as exc:
            raise ValueError(
                f"RAG_CORE_PUBLIC_KEYS entry {kid!r} is not a base64 raw Ed25519 public key"
            ) from exc
        if len(raw) != ED25519_PUBLIC_KEY_BYTES:
            raise ValueError(f"RAG_CORE_PUBLIC_KEYS entry {kid!r} is not a 32-byte Ed25519 public key")
        keys[kid] = Ed25519PublicKey.from_public_bytes(raw)
    return keys


def _string_claim(claims: Mapping[str, Any], name: str) -> str:
    value = claims.get(name)
    if not isinstance(value, str) or not value:
        raise TokenRejectedError(REASON_MISSING_CLAIM)
    return value


def _optional_kb(claims: Mapping[str, Any]) -> str | None:
    value = claims.get(CLAIM_KB)
    if value is None:
        return None
    try:
        return str(uuid.UUID(str(value)))
    except ValueError:
        raise TokenRejectedError(REASON_INVALID_CLAIM) from None


class AppTokenVerifier:
    def __init__(
        self,
        public_keys: Mapping[str, Ed25519PublicKey],
        issuer: str,
        audience: str,
        leeway_seconds: int,
        max_lifetime_seconds: int,
        clock: Callable[[], float] = time.time,
    ) -> None:
        self._keys = dict(public_keys)
        self._issuer = issuer
        self._audience = audience
        self._leeway = leeway_seconds
        self._max_lifetime = max_lifetime_seconds
        self._clock = clock

    def verify(self, token: str) -> Caller:
        key = self._key_for(token)
        claims = self._decode(token, key)
        self._check_times(claims)
        return self._caller(claims)

    def _key_for(self, token: str) -> Ed25519PublicKey:
        try:
            header = jwt.get_unverified_header(token)
        except jwt.DecodeError:
            raise TokenRejectedError(REASON_MALFORMED) from None
        if header.get("alg") != ALGORITHM:
            raise TokenRejectedError(REASON_WRONG_ALGORITHM)
        if header.get("typ") != TOKEN_TYPE:
            raise TokenRejectedError(REASON_WRONG_TYPE)
        key = self._keys.get(str(header.get("kid")))
        if key is None:
            raise TokenRejectedError(REASON_UNKNOWN_KID)
        return key

    def _decode(self, token: str, key: Ed25519PublicKey) -> dict[str, Any]:
        # Time checks are done below against the injected clock, so the vector's fixed "now" is testable.
        options: Any = {
            "verify_exp": False,
            "verify_iat": False,
            "verify_nbf": False,
            "require": list(REQUIRED_REGISTERED_CLAIMS),
        }
        try:
            return jwt.decode(
                token,
                key,
                algorithms=[ALGORITHM],
                audience=self._audience,
                issuer=self._issuer,
                options=options,
            )
        except jwt.InvalidSignatureError:
            raise TokenRejectedError(REASON_BAD_SIGNATURE) from None
        except jwt.MissingRequiredClaimError:
            raise TokenRejectedError(REASON_MISSING_CLAIM) from None
        except jwt.InvalidAudienceError:
            raise TokenRejectedError(REASON_WRONG_AUDIENCE) from None
        except jwt.InvalidIssuerError:
            raise TokenRejectedError(REASON_WRONG_ISSUER) from None
        except jwt.InvalidTokenError:
            raise TokenRejectedError(REASON_MALFORMED) from None

    def _check_times(self, claims: Mapping[str, Any]) -> None:
        issued, expires = claims.get(CLAIM_ISSUED_AT), claims.get(CLAIM_EXPIRES)
        if not isinstance(issued, int) or not isinstance(expires, int):
            raise TokenRejectedError(REASON_INVALID_CLAIM)
        now = self._clock()
        if expires <= now - self._leeway:
            raise TokenRejectedError(REASON_EXPIRED)
        if issued > now + self._leeway:
            raise TokenRejectedError(REASON_NOT_YET_VALID)
        if expires - issued > self._max_lifetime:
            raise TokenRejectedError(REASON_LIFETIME_TOO_LONG)

    def _caller(self, claims: Mapping[str, Any]) -> Caller:
        org_id = _string_claim(claims, CLAIM_ORG)
        scope = _string_claim(claims, CLAIM_SCOPE)
        request_id = _string_claim(claims, CLAIM_REQUEST_ID)
        jti = _string_claim(claims, CLAIM_JTI)
        if ORG_ID_PATTERN.fullmatch(org_id) is None or not is_valid_request_id(request_id):
            raise TokenRejectedError(REASON_INVALID_CLAIM)
        return Caller(org_id=org_id, scope=scope, kb_id=_optional_kb(claims), request_id=request_id, jti=jti)
