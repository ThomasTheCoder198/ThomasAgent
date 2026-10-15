"""Dev-only: ask a ragx KB tool the way core will in M2 (App token with scope tool:<name> and claim kb).

  uv run --directory ragx python ../scripts/kb_ask.py --org <orgId> --kb <kbId> "Câu hỏi?"
  uv run --directory ragx python ../scripts/kb_ask.py --org <orgId> --kb <kbId> --tool kb_list_documents
Prints results only: labels, file, locator and block text. Never prints the token.
"""

import argparse
import io
import json
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Any

import jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

DEFAULT_KEY = Path(__file__).resolve().parents[1] / "deploy" / "ragx" / "dev-app-signing-key.pem"
DEFAULT_URL = "http://localhost:8090"
TOKEN_TYPE = "thomas-app+jwt"  # noqa: S105  # JWT header type, not a credential.
DEFAULT_ISSUER = "thomas-core"
DEFAULT_AUDIENCE = "ragx"
DEFAULT_KID = "dev-1"
TOKEN_TTL_SECONDS = 60
HTTP_TIMEOUT_SECONDS = 60
SEARCH_TOOL = "kb_search"
TRACE_ID_HEADER = "X-Ragx-Trace-Id"
BLOCK_PREVIEW_CHARS = 300
# These mirror ragx (auth/caller.py TOOL_SCOPE_PREFIX, platform/request_id.py REQUEST_ID_HEADER).
TOOL_SCOPE_PREFIX = "tool:"
REQUEST_ID_PREFIX = "req_"
REQUEST_ID_HEADER = "X-Request-Id"
AUTHORIZATION_HEADER = "Authorization"
CONTENT_TYPE_HEADER = "Content-Type"
JSON_CONTENT_TYPE = "application/json"
EXIT_OK = 0
EXIT_FAILED = 1
EXIT_UNUSABLE = 2


def _token(key_path: Path, org: str, kb: str, tool: str, rid: str) -> str:
    key = serialization.load_pem_private_key(key_path.read_bytes(), password=None)
    if not isinstance(key, Ed25519PrivateKey):
        raise SystemExit("the key file is not an Ed25519 private key")
    now = int(time.time())
    claims = {
        "iss": DEFAULT_ISSUER,
        "aud": DEFAULT_AUDIENCE,
        "oid": org,
        "kb": kb,
        "scp": f"{TOOL_SCOPE_PREFIX}{tool}",
        "rid": rid,
        "jti": str(uuid.uuid4()),
        "iat": now,
        "exp": now + TOKEN_TTL_SECONDS,
    }
    return jwt.encode(claims, key, algorithm="EdDSA", headers={"kid": DEFAULT_KID, "typ": TOKEN_TYPE})


def _print_search(data: dict[str, Any]) -> None:
    if data["insufficientEvidence"]:
        print("(không đủ bằng chứng / insufficient evidence)")
    for result in data["results"]:
        section = " > ".join(result["section"])
        print(f"\n#{result['source']}  {result['fileName']}  score={result['score']:.3f}  {section}")
        for block in result["blocks"]:
            print(f"  {block['label']} {json.dumps(block['locator'], ensure_ascii=False)}")
            print(f"     {block['text'][:BLOCK_PREVIEW_CHARS]}")


def _read_error(exc: urllib.error.HTTPError) -> str:
    raw = exc.read().decode(errors="replace")
    try:
        return json.dumps(json.loads(raw), ensure_ascii=False, indent=1)
    except ValueError:
        return f"HTTP {exc.code}: {raw}"


def _fail(message: str, rid: str | None = None, code: int = EXIT_FAILED) -> int:
    suffix = f" (request_id={rid})" if rid else ""
    print(f"kb_ask: {message}{suffix}", file=sys.stderr)
    return code


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "question", nargs="?", default="", help=f"the query; used only with --tool {SEARCH_TOOL}"
    )
    parser.add_argument("--org", required=True)
    parser.add_argument("--kb", required=True, help="a KB of --org; any other KB answers KB_NOT_FOUND")
    parser.add_argument("--tool", default=SEARCH_TOOL)
    parser.add_argument("--body", help=f"raw JSON body, needed for tools other than {SEARCH_TOOL}")
    parser.add_argument("--key", type=Path, default=DEFAULT_KEY)
    parser.add_argument("--url", default=DEFAULT_URL)
    return parser.parse_args()


def _request_body(args: argparse.Namespace) -> dict[str, Any] | None:
    """None when the arguments are unusable; the caller reports it."""
    if args.body is not None and args.question:
        return None
    if args.body is not None:
        parsed = json.loads(args.body)
        return parsed if isinstance(parsed, dict) else None
    return {"query": args.question} if args.tool == SEARCH_TOOL else {}


def _print_payload(tool: str, payload: Any) -> None:
    data = payload["data"]
    if tool == SEARCH_TOOL:
        _print_search(data)
    else:
        print(json.dumps(data, ensure_ascii=False, indent=1))


def _post(args: argparse.Namespace, body: dict[str, Any], rid: str, token: str) -> int:
    request = urllib.request.Request(  # noqa: S310  # dev tool, local URL
        f"{args.url}/v1/tools/{args.tool}",
        data=json.dumps(body).encode(),
        method="POST",
        headers={
            AUTHORIZATION_HEADER: f"Bearer {token}",
            REQUEST_ID_HEADER: rid,
            CONTENT_TYPE_HEADER: JSON_CONTENT_TYPE,
        },
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT_SECONDS) as response:  # noqa: S310
            payload = json.load(response)
            trace = response.headers.get(TRACE_ID_HEADER)
        elapsed_ms = round((time.perf_counter() - started) * 1000)
        print(f"request_id={rid} trace_id={trace} {elapsed_ms} ms")
        _print_payload(args.tool, payload)
    except urllib.error.HTTPError as exc:
        print(_read_error(exc), file=sys.stderr)
        return _fail(f"HTTP {exc.code}", rid)
    except (urllib.error.URLError, TimeoutError, ConnectionError) as exc:
        reason = getattr(exc, "reason", exc)
        return _fail(f"ragx not reachable at {args.url} ({reason}); is `task up:ragx` running?", rid)
    except (ValueError, KeyError, TypeError):
        return _fail("ragx answered something that is not a tool response", rid)
    return EXIT_OK


def main() -> int:
    # Vietnamese output and help text crash on the Windows default console codepage otherwise.
    for stream in (sys.stdout, sys.stderr):
        if isinstance(stream, io.TextIOWrapper):
            stream.reconfigure(encoding="utf-8")
    args = _parse_args()
    try:
        body = _request_body(args)
    except ValueError:
        body = None
    if body is None:
        return _fail("pass either a question or --body (a JSON object), not both", code=EXIT_UNUSABLE)
    if args.tool == SEARCH_TOOL and "query" not in body:
        return _fail(f"{SEARCH_TOOL} needs a question or a --body with a query", code=EXIT_UNUSABLE)
    rid = f"{REQUEST_ID_PREFIX}{uuid.uuid4()}"
    try:
        token = _token(args.key, args.org, args.kb, args.tool, rid)
    except OSError as exc:
        return _fail(
            f"cannot read the signing key {args.key} ({exc.strerror}); run `task ragx:dev-key`",
            code=EXIT_UNUSABLE,
        )
    except ValueError:
        return _fail(f"{args.key} is not a PEM private key", code=EXIT_UNUSABLE)
    return _post(args, body, rid, token)


if __name__ == "__main__":
    sys.exit(main())
