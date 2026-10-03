# Contracts

## Purpose
Single source for error codes and shared schemas used by the Go core, Python RAG app, and frontend. Owns catalog validation and code generation.

## Entry points
- `errors.yaml` — authoritative error codes, HTTP statuses, retryability, and VI/EN messages.
- `cmd/errorcodegen` — command invoked by `task gen`.
- `internal/errorcodegen` — catalog parser and Go, Python, and TypeScript renderers.

## Dependencies
- Uses: `gopkg.in/yaml.v3` v3.0.1; tests use `github.com/stretchr/testify` v1.12.1.
- Used by: `backend-core`, `ragx`, and `frontend` through generated catalogs.

## Run & test
From the repository root, with Go 1.27.1 on PATH:
```bash
task gen
task test:contracts
```
For a fresh test run from this folder:
```bash
go mod tidy
go test ./... -count=1
go vet ./...
```

## Conventions
Never edit generated files. Add an error code to `errors.yaml` first, then run `task gen`.
Catalog version is 1; codes use UPPER_SNAKE and must be unique. HTTP statuses are 400–599 and both localized messages are required.
The parser sorts entries by code so generated output is deterministic. Renderers emit UTF-8 with LF line endings.
Three files are generated: `backend-core/internal/errors/errors.go` (canonical Go definitions),
`ragx/src/thomas_ragx/platform/error_codes_gen.py`, and `frontend/lib/errors/codes.gen.ts`.
The canonical Go file defines immutable named errors such as `errors.ErrNotFound` and
maps codes to `ErrorDefinition{HTTPStatus, Retryable, MessageVI, MessageEN}` through `LookupDefinition`.
There is no second Go message catalog or mutable global error instance.
Python exposes `ErrorDefinition.http_status` and `ERROR_DEFINITIONS`; TypeScript exposes
`errorDefinitions` with `httpStatus`. See the [glossary](../docs/glossary.md) for shared terminology.

## Common failures
- Invalid catalog → malformed YAML, duplicate code, unsupported version, invalid status, or missing message → correct `errors.yaml` and rerun generation.
- Golden mismatch → output differs from the fixtures → inspect the diff; regenerate an intentional change with the generator tests described in [the renderer README](internal/errorcodegen/README.md).
- Missing dependency checksum → dependencies have not been resolved → run `go mod tidy`.
