# Scripts

## Purpose
Owns root service smoke checks, including `ragx-health` and `error-response-404`.
See the [glossary](../docs/glossary.md) for response and health terminology.

## Entry points
- `smoke.sh`: called by `task smoke`.
- `smoke_identity.py`: checks login, identity, CSRF, provider creation and cleanup, then logout.

## Dependencies
- Uses Bash, curl and uv-managed Python with python-dotenv; requires the full compose stack.
- Used by local deployment verification.

## Run & test
```bash
task up
task smoke
bash -n scripts/smoke.sh
```

## Conventions
Keep shell files LF and fail immediately when any probe fails.
Identity probes load the configured owner credentials in memory, preserve the cookie jar,
and print only check names and status codes. Temporary smoke providers are deleted;
the login session is logged out on success. No model roles are changed.

## Common failures
- Connection refused: start the stack and inspect failed container logs.
- Langfuse health failure: wait for its initial database migrations to finish.
