# Error catalog generator command

## Purpose
Reads the shared catalog and writes canonical Go errors, Go compatibility aliases,
Python and TypeScript error definitions.

## Entry points
- `main.go`: parses input and output flags, validates the catalog, and writes outputs.

## Dependencies
- Uses: contracts/internal/errgen and the Go standard library.
- Used by: root task gen.

## Run & test
```bash
task gen
task test:contracts
```

## Conventions
Output paths are supplied by Taskfile.yml. Generated files must never be edited manually.
`-go` selects `internal/errors/errors.go`; `-go-compat` selects legacy `apperr` aliases.
`-py` and `-ts` select the other consumer outputs. Catalog validation completes before outputs are written.

## Common failures
- Invalid catalog: fix contracts/errors.yaml and regenerate.
- Output write failure: check destination permissions.
