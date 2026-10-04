# Vault

## Purpose
Encrypts secrets with AES-256-GCM and persists encrypted rows. Rotation and re-encryption jobs are out of scope.

## Entry points
- `NewCipher(keyID, masterKeyBase64)` constructs the cipher from base64 of exactly 32 bytes.
- `Seal` and `Open` encrypt and authenticate bytes using caller-supplied AAD.
- `NewStore`, `Put`, `Replace`, `Get`, `Delete` manage secret rows through `postgres.DBTX`.

## Dependencies
- Uses: Go crypto, google UUID, and `postgres.DBTX` (pool or transaction).
- Used by: identity and registry features in subsequent tasks.

## Run & test
```bash
cd backend-core
go test ./internal/vault/... -count=1 -p 2
```
Docker Desktop is required for PostgreSQL integration tests.

## Conventions
The secret UUID bytes are AAD, so copying encrypted values into another row fails authentication. Every seal uses a fresh random nonce. Key IDs are stored as metadata; key rotation needs a future re-encrypt job. Never log plaintext or master keys. Store errors are returned for callers to handle at the boundary.

Secrets are scoped to the [Platform tenant](../../../docs/glossary.md#thomasagent-glossary) (`default`). M0.2 relies on the database column default; from M1, repositories take `tenant_id` from context. `Get` and `Replace` return `ErrSecretNotFound` for a missing row; `Delete` is idempotent.

## Common failures
- Invalid master key: generate a base64 32-byte value with `openssl rand -base64 32`.
- `ErrDecrypt`: wrong key, wrong row AAD, malformed nonce, or tampered encrypted data.
- `ErrSecretNotFound`: `Get` or `Replace` could not find the secret; check with `errors.Is`.
- Database failure: the store returns the wrapped database error.
