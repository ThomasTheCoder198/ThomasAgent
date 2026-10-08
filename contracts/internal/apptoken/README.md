# App-token test vector

## Purpose

Builds the deterministic App-token test vector (`contracts/app-token.json`) that core's signer and ragx's verifier both test against.

## Entry points

- `Build` — constructs the vector cases.
- `Render` — emits the exact JSON file content.
- `cmd/apptokengen` — writes the vector during `task gen`.

## Dependencies

- Uses: standard library only.
- Used by: `task gen`, ragx tests (Task 4), and core signer tests (M1.2).

## Run & test

```bash
go test ./internal/apptoken/... -count=1
```

## Conventions

Keys come from SHA-256 of a fixed label, so the file never changes unless the code does. The private seed is test-only and must never be used outside tests. Reason strings are the contract for rejection logging.

## Common failures

- Stale vector → run `task gen`.
