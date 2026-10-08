# Auth

## Purpose

Verifies core's App token (EdDSA JWT, 60 s) with pinned public keys and turns it into a `Caller`. Ragx never fetches JWKS and never calls core.

## Entry points

- `AppTokenVerifier.verify` validates a token and returns its signed caller fields.
- `require_scope` enforces a route's required scope.
- `ProvisionCaller`, `KbReadCaller`, and `KbWriteCaller` are FastAPI route dependencies.

## Dependencies

- Uses: PyJWT 2.15.0, cryptography 50.0.1, and `RAG_CORE_PUBLIC_KEYS`, `RAG_APP_TOKEN_ISSUER`, `RAG_APP_TOKEN_AUDIENCE`, `RAG_APP_TOKEN_LEEWAY_SECONDS`, `RAG_APP_TOKEN_MAX_LIFETIME_SECONDS`.
- Used by: `/v1` API routes.

## Run & test

```bash
cd ragx
uv run pytest -q tests/unit/test_app_token.py tests/unit/test_app_token_dependency.py
```

## Conventions

`contracts/app-token.json` is the contract: every reason string there must be produced exactly. The signed `rid` must match the accepted `X-Request-Id`; a missing or mismatched header is rejected as `invalid_claim`. Rejections log `app token rejected` with the reason only; the token, claims, and key are never logged. The org comes only from `oid`; a tool's KB only from `kb`. During key rotation, list both kids in `RAG_CORE_PUBLIC_KEYS` until core has signed with the new key for longer than the token lifetime.

## Common failures

- `unknown_kid` → `RAG_CORE_PUBLIC_KEYS` lacks core's current kid → update the pinned keys.
- `expired` or `not_yet_valid` on every call → server clocks differ by more than the leeway → synchronize clocks with NTP.
