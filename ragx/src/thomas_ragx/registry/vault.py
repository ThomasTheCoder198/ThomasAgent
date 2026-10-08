import base64
import binascii
import os
from dataclasses import dataclass
from typing import Final
from uuid import UUID

import structlog
from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from pydantic import SecretStr

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError

MASTER_KEY_BYTES: Final = 32
NONCE_BYTES: Final = 12
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class Sealed:
    nonce: bytes
    ciphertext: bytes


def provider_key_aad(provider_id: UUID) -> bytes:
    # Binding the ciphertext to its row stops a copied ciphertext from decrypting under another provider.
    return f"provider:{provider_id}:api_key".encode()


class Vault:
    def __init__(self, master_key: SecretStr) -> None:
        try:
            key = base64.b64decode(master_key.get_secret_value(), validate=True)
        except (binascii.Error, ValueError):
            raise ValueError("RAG_VAULT_MASTER_KEY must be base64") from None
        if len(key) != MASTER_KEY_BYTES:
            raise ValueError("RAG_VAULT_MASTER_KEY must decode to exactly 32 bytes")
        self._aead = AESGCM(key)

    def seal(self, plaintext: str, aad: bytes) -> Sealed:
        nonce = os.urandom(NONCE_BYTES)
        return Sealed(nonce=nonce, ciphertext=self._aead.encrypt(nonce, plaintext.encode(), aad))

    def open(self, sealed: Sealed, aad: bytes) -> SecretStr:
        try:
            return SecretStr(self._aead.decrypt(sealed.nonce, sealed.ciphertext, aad).decode())
        except InvalidTag:
            # Wrong or rotated master key, or a ciphertext moved between rows. Nothing secret is logged.
            log.error("vault decrypt failed")  # noqa: TRY400 - traceback may expose ciphertext or keys
            raise AppError(ErrorCode.INTERNAL_ERROR) from None
