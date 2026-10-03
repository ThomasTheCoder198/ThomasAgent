# Scripts

## Purpose
Owns root service smoke checks, including `ragx-health` and `error-response-404`.
See the [glossary](../docs/glossary.md) for response and health terminology.

## Entry points
- `smoke.sh`: called by `task smoke`.

## Dependencies
- Uses Bash and curl; requires the full compose stack.
- Used by local deployment verification.

## Run & test
```bash
task up
task smoke
bash -n scripts/smoke.sh
```

## Conventions
Keep shell files LF and fail immediately when any probe fails.

## Common failures
- Connection refused: start the stack and inspect failed container logs.
- Langfuse health failure: wait for its initial database migrations to finish.
