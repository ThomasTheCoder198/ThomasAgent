# Vault

## Purpose
Encrypts secrets with AES-256-GCM and persists encrypted rows. Rotation and re-encryption jobs are out of scope.

## Entry points
- `NewCipher(keyID, masterKeyBase64)` constructs the cipher from base64 of exactly 32 bytes.
- `Seal` and `Open` encrypt and authenticate bytes using caller-supplied AAD.
- `NewStore`, `Put`, `Replace`, `Get`, `Delete` manage secret rows through `postgres.DBTX`.

## Dependencies
- Uses: Go crypto, google UUID, and `postgres.DBTX` (pool or transaction).
- Used by: provider registry services for API-key storage and resolution.

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/vault/... -count=1 -p 2
```
Docker Desktop is required for PostgreSQL integration tests.

## Conventions
The trusted tenant, secret UUID and key ID form the AES-GCM AAD, so copying encrypted values into another tenant or row fails authentication. Reads reject mismatched key IDs. Every seal uses a fresh random nonce. Key rotation needs a future re-encrypt job. Never log plaintext or master keys. Store errors are returned for callers to handle at the boundary.

Secrets require a nonempty tenant in trusted server context; a missing tenant returns the catalog `INTERNAL_ERROR` before any query or encryption. AES-GCM additional authenticated data binds tenant ID, secret ID and key ID, and reads reject a sealed key ID that differs from the active cipher. `Get`, `Replace` and `Delete` return `ErrSecretNotFound` when the row is absent in the trusted tenant, including attempted cross-tenant access.

## Common failures
- Invalid master key: generate a base64 32-byte value with `openssl rand -base64 32`.
- `ErrDecrypt`: wrong key, wrong row AAD, malformed nonce, or tampered encrypted data.
- `ErrSecretNotFound`: `Get` or `Replace` could not find the secret; check with `errors.Is`.
- Database failure: the store returns the wrapped database error.

New writes and replacements use `aad_version=3`: each UTF-8 field (tenant ID, canonical UUID string, key ID) is prefixed with its byte length as an unsigned varint. Colons in IDs cannot shift field boundaries. `Get` reads version 1 UUID-only AAD and version 2 colon-bound AAD only when explicitly marked, then reseals either as version 3. The update compares the old version, nonce and ciphertext to preserve concurrent replacements. Version 3 reads are stable and do not reseal. Failed authentication never falls back to another format; unknown versions return `ErrDecrypt`.

`20261005000001_vault_aad_version.sql` expands the format constraint without changing applied migrations. Its Down preserves the expanded constraint and format markers because SQL cannot reseal ciphertext without the master key. Schema Down/Up is safe with the current binary; older binaries cannot read newly sealed version 3 rows and require a pre-upgrade encrypted-data backup or authenticated reseal before an application downgrade.
