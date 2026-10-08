"""Dev-only App-token tool. Core signs App tokens from M1.2; until then this lets smoke and humans call ragx.

Run with ragx's environment (PyJWT, cryptography):
  uv run --directory ragx python ../scripts/app_token_dev.py keygen --out ../deploy/ragx/dev-app-signing-key.pem
  uv run --directory ragx python ../scripts/app_token_dev.py mint --key ../deploy/ragx/dev-app-signing-key.pem \
      --org smoke-org --scope org:provision --rid req_example-0001
ragx rejects a token whose `rid` differs from the request's X-Request-Id, so send the same id in that header.
"""

import argparse
import base64
import json
import os
import sys
import time
import uuid
from pathlib import Path

import jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

TOKEN_TYPE = "thomas-app+jwt"
DEFAULT_ISSUER = "thomas-core"
DEFAULT_AUDIENCE = "ragx"
DEFAULT_KID = "dev-1"
DEFAULT_TTL_SECONDS = 60
PRIVATE_FILE_MODE = 0o600


def keygen(args: argparse.Namespace) -> int:
    path = Path(args.out)
    if path.exists() and not args.force:
        print(f"{path} exists; pass --force to replace it (ragx must then get the new public key)", file=sys.stderr)
        return 1
    key = Ed25519PrivateKey.generate()
    path.write_bytes(
        key.private_bytes(
            serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()
        )
    )
    os.chmod(path, PRIVATE_FILE_MODE)
    public = key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
    keys = json.dumps({args.kid: base64.b64encode(public).decode()}, separators=(",", ":"))
    print(f"RAG_CORE_PUBLIC_KEYS={keys}")
    return 0


def mint(args: argparse.Namespace) -> int:
    key = serialization.load_pem_private_key(Path(args.key).read_bytes(), password=None)
    if not isinstance(key, Ed25519PrivateKey):
        print("the key file is not an Ed25519 private key", file=sys.stderr)
        return 1
    now = int(time.time())
    claims: dict[str, object] = {
        "iss": args.issuer,
        "aud": args.audience,
        "oid": args.org,
        "scp": args.scope,
        "rid": args.rid or f"req_{uuid.uuid4()}",
        "jti": str(uuid.uuid4()),
        "iat": now,
        "exp": now + args.ttl,
    }
    if args.kb:
        claims["kb"] = args.kb
    print(jwt.encode(claims, key, algorithm="EdDSA", headers={"kid": args.kid, "typ": TOKEN_TYPE}))
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command", required=True)
    generate = commands.add_parser("keygen", help="create a dev signing key and print RAG_CORE_PUBLIC_KEYS")
    generate.add_argument("--out", required=True)
    generate.add_argument("--kid", default=DEFAULT_KID)
    generate.add_argument("--force", action="store_true")
    generate.set_defaults(handler=keygen)
    token = commands.add_parser("mint", help="print one App token")
    token.add_argument("--key", required=True)
    token.add_argument("--kid", default=DEFAULT_KID)
    token.add_argument("--org", required=True)
    token.add_argument("--scope", required=True)
    token.add_argument("--kb")
    token.add_argument("--rid", help="request id; must equal the X-Request-Id sent with the token")
    token.add_argument("--ttl", type=int, default=DEFAULT_TTL_SECONDS)
    token.add_argument("--issuer", default=DEFAULT_ISSUER)
    token.add_argument("--audience", default=DEFAULT_AUDIENCE)
    token.set_defaults(handler=mint)
    args = parser.parse_args()
    return int(args.handler(args))


if __name__ == "__main__":
    sys.exit(main())
