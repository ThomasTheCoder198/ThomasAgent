# Error catalog rendering

## Purpose
Owns catalog validation and deterministic source rendering for the three consumers.

## Entry points
- `Parse`: validates catalog version, codes, statuses, and messages.
- `RenderGo`: canonical named Go errors and readable definition lookup.
- `RenderPython`, `RenderTS`: produce generated source for other consumers.
- `errorcodegen_test.go`: golden output and invalid-catalog cases.

## Dependencies
- Uses: yaml.v3 and Go formatting; testify in tests.
- Used by: contracts/cmd/errorcodegen.

## Run & test
```bash
task test:contracts
task gen
```

## Conventions
Renderers emit LF and use catalog entries sorted by code. Golden fixtures in testdata are test inputs, not modules.
Go named error constants cannot carry per-request state. The generated lookup switch supplies metadata
without a mutable global map. Only one Go definition file is generated.
See the [glossary](../../../docs/glossary.md) for shared error terminology.
For an intentional renderer change, regenerate golden files through the renderer tests from `contracts/`:
```bash
UPDATE_GOLDEN=1 go test ./internal/errorcodegen -run MatchesGolden -count=1
```
Then run the complete contracts tests without `UPDATE_GOLDEN` to verify the fixtures.

## Common failures
- Golden mismatch: inspect intended catalog or renderer changes before updating fixtures.
- Invalid catalog: supply unique uppercase codes and both localized messages.
