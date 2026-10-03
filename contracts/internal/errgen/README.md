# Error catalog rendering

## Purpose
Owns catalog validation and deterministic source rendering for the three consumers.

## Entry points
- `Parse`: validates catalog version, codes, statuses, and messages.
- `RenderGo`: canonical named Go errors and readable definition lookup.
- `RenderGoCompatibility`: aliases for the previous `platform/apperr` code API.
- `RenderPython`, `RenderTS`: produce generated source for other consumers.
- `errgen_test.go`: golden output and invalid-catalog cases.

## Dependencies
- Uses: yaml.v3 and Go formatting; testify in tests.
- Used by: contracts/cmd/errgen.

## Run & test
```bash
task test:contracts
task gen
```

## Conventions
Renderers emit LF and use catalog entries sorted by code. Golden fixtures in testdata are test inputs, not modules.
Go sentinel constants cannot carry per-request state. The generated lookup switch supplies metadata
without a mutable global map, and compatibility output never duplicates messages or codes.

## Common failures
- Golden mismatch: inspect intended catalog or renderer changes before updating fixtures.
- Invalid catalog: supply unique uppercase codes and both localized messages.
