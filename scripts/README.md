# Scripts

## Purpose
Owns root service smoke checks, including core log/metrics correlation, optional ragx/Phoenix health and `error-response-404`.
See the [glossary](../docs/glossary.md) for response and health terminology.

## Entry points
- `smoke.sh`: called by `task smoke`; checks ragx readiness, idempotent org provisioning when a dev signing key exists.
- `app_token_dev.py`: dev-only keygen/mint of App tokens until core signs them in M1.2. Ragx rejects a token whose `rid` differs from the request's `X-Request-Id` (spec §3), so pass `mint --rid <id>` and send the same id in that header.
- `smoke_ragx_app.py`: end to end through core's ragx proxy as a browser would: org ready, list KBs, presign, MinIO CORS preflight, MinIO POST, confirm, presigned download, cleanup (checks `ragx-app-*`, `minio-cors-preflight`, `minio-post`). It reuses one org named `Smoke` (orgs cannot be deleted yet) and deletes its file. `smoke.sh` runs it only when ragx is up and `deploy/ragx/dev-app-signing-key.pem` exists. `SMOKE_WEB_ORIGIN` (default `http://localhost:3000`) is the Origin sent to MinIO and must match `MINIO_API_CORS_ALLOW_ORIGIN`. Deferred: a check that the ragx worker consumed the upload (covered by ragx tests).
- `smoke_identity.py`: checks login, identity, CSRF, provider creation and cleanup, then logout.

## Dependencies
- Uses Bash, curl and uv-managed Python with python-dotenv; requires `deploy/core`, with `deploy/ragx` checked when its API container exists.
- Used by local deployment verification.

## Run & test
```bash
task up
# Optional: start ragx and Phoenix for combined-stack checks.
task up:ragx
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
- Ragx health failure: start or repair `deploy/ragx`; smoke skips it only when the API container is absent.
- Phoenix health failure: inspect Phoenix startup logs and `PHOENIX_HOST_PORT` in `deploy/ragx/.env`.
