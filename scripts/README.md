# Scripts

## Purpose
Owns root service smoke checks, including core log/metrics correlation, optional ragx/Phoenix health and `error-response-404`, and the dev-only helpers that call ragx by hand (`app_token_dev.py`, `kb_ask.py`).
See the [glossary](../docs/glossary.md) for response and health terminology.

## Entry points
- `smoke.sh`: called by `task smoke`; checks ragx readiness, idempotent org provisioning when a dev signing key exists.
- `app_token_dev.py`: dev-only keygen/mint of App tokens until core signs them in M1.2. Ragx rejects a token whose `rid` differs from the request's `X-Request-Id` (spec §3), so pass `mint --rid <id>` and send the same id in that header.
- `kb_ask.py`: dev-only; asks a ragx KB tool the way core will in M2. It signs a 60 s App token (scope `tool:<name>`, claims `oid` and `kb`, `rid` equal to the `X-Request-Id` it sends) with the dev signing key (`--key` overrides) and POSTs `/v1/tools/<name>` (`--url`, default `http://localhost:8090`). The token never prints.
  - `--kb` must be a KB of `--org`: ragx takes the org only from `oid` and the KB only from `kb`, so a KB of another org answers `KB_NOT_FOUND` (the isolation rule).
  - Output: the request id, the `X-Ragx-Trace-Id` (to find the trace in Phoenix) and the latency; then, for `kb_search`, each result's `[n.k]` labels, file, score, section, locator and block text; for other tools `data` as JSON.
  - Arguments: the positional question is for `kb_search` only; `--body '<json object>'` replaces it and is required for the other tools that take input (`{"fileId": "..."}` for `kb_outline`, `{"nodeId": "..."}` for `kb_read_section`). Passing both is rejected.
  - Failures print one `kb_ask:` line on stderr and no traceback: exit 1 for an HTTP error (the error body is printed first), ragx unreachable or an unexpected response (with the request id); exit 2 for unusable arguments or a missing or invalid key. `kb_search` also needs a rerank role, else `LLM_ROLE_NOT_ASSIGNED`.
- `smoke_ragx_app.py`: end to end through core's ragx proxy as a browser would: org ready, list KBs, presign, MinIO CORS preflight, MinIO POST, confirm, presigned download, cleanup (checks `ragx-app-*`, `minio-cors-preflight`, `minio-post`). It reuses one org named `Smoke` (orgs cannot be deleted yet) and deletes its file. `smoke.sh` runs it only when ragx is up and `deploy/ragx/dev-app-signing-key.pem` exists. `SMOKE_WEB_ORIGIN` (default `http://localhost:3000`) is the Origin sent to MinIO and must match `MINIO_API_CORS_ALLOW_ORIGIN`. Deferred: a check that the ragx worker consumed the upload (covered by ragx tests).
- `smoke_identity.py`: checks login, identity, CSRF, provider creation and cleanup, then logout.

## Dependencies
- Uses Bash, curl and uv-managed Python with python-dotenv; requires `deploy/core`, with `deploy/ragx` checked when its API container exists.
- `app_token_dev.py` and `kb_ask.py` run under ragx's uv environment (`uv run --directory ragx python ../scripts/<file>`: PyJWT, cryptography) and need the dev signing key `deploy/ragx/dev-app-signing-key.pem` (`task ragx:dev-key`); `kb_ask.py` also needs the ragx API up (`task up:ragx`).
- Used by local deployment verification.

## Run & test
```bash
task up
# Optional: start ragx and Phoenix for combined-stack checks.
task up:ragx
task smoke
bash -n scripts/smoke.sh
# From ragx/: ask a KB tool, and lint the script like the other Python scripts.
uv run python ../scripts/kb_ask.py --org <orgId> --kb <kbId> "question"
uv run python ../scripts/kb_ask.py --org <orgId> --kb <kbId> --tool kb_list_documents
uv run ruff check ../scripts/kb_ask.py && uv run ruff format --check ../scripts/kb_ask.py
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
