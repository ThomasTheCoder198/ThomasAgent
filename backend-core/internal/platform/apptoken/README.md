# App token

## Purpose
Signs the per-call App token core sends to ragx (spec §3): EdDSA, `typ thomas-app+jwt`, 60 s. It does not call ragx; `internal/platform/ragx` does.

## Entry points
- `NewAppTokenSigner` and `Signer.Sign`: build one token per call.
- `LoadSigningKey`: reads the PKCS#8 PEM key from `CORE_APP_SIGNING_KEY` or `CORE_APP_SIGNING_KEY_FILE`.
- `ScopeOrgProvision`, `ScopeKBRead`, `ScopeKBWrite`: one scope per call.

## Dependencies
- Uses: `golang-jwt/jwt/v5`, `config.AppConfig`, `requestid`, `tenant`.
- Used by: `internal/platform/ragx` (Task 4).

## Run & test
```bash
cd backend-core
go test ./internal/platform/apptoken/... -count=1
```

## Conventions
- `oid` comes only from trusted tenant context, and the platform tenant can never be signed.
- `rid` must equal the `X-Request-Id` sent with the call (the ragx client takes both from one context value).
- The payload is byte-identical to `contracts/app-token.json`. The header is compared as JSON, because golang-jwt sorts header keys.
- Keys are PKCS#8 PEM, from a value or a mounted file. Errors never contain key material.

## Common failures
- `ErrAppSigningKeyMissing` at startup → set `CORE_APP_SIGNING_KEY_FILE` (dev: `deploy/ragx/dev-app-signing-key.pem` from `task ragx:dev-key`).
- ragx answers 401 `APP_TOKEN_INVALID` (`unknown_kid`) → `CORE_APP_SIGNING_KEY_ID` is not in ragx's `RAG_CORE_PUBLIC_KEYS`.
