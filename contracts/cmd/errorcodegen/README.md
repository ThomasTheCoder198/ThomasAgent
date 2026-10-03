# Error catalog generator command

## Purpose
Reads the shared catalog and writes Go, Python and TypeScript error definitions.

## Entry points
- `main.go`: parses input and output flags, validates the catalog, and writes outputs.

## Dependencies
- Uses: contracts/internal/errorcodegen and the Go standard library.
- Used by: root task gen.

## Run & test
```bash
task gen
task test:contracts
```

## Conventions
Output paths are supplied by Taskfile.yml. Generated files must never be edited manually.
Shared naming follows the [glossary](../../../docs/glossary.md).
`-go` selects `internal/errors/errors.go`.
`-py` and `-ts` select the other consumer outputs. Catalog validation completes before outputs are written.

## Common failures
- Invalid catalog: fix contracts/errors.yaml and regenerate.
- Output write failure: check destination permissions.
