# Admin account

## Purpose
The single ragx UI admin: account, argon2id passwords, hashed sessions, login limits. It does not authorize `/v1` calls; those use core's App token.

## Entry points
- `AdminService` — login, session lookup and logout for `ui/routes.py`.
- `ensure_admin_account` — called once by `ragx bootstrap`.
- `LoginLimiter` — failed-login budget per hashed email and per IP.

## Dependencies
- Uses: argon2-cffi, ragx Postgres (`admin_users`, `ui_sessions`, migration `0002_admin`), ragx Redis (attempt counters).
- Used by: `thomas_ragx.ui`, `thomas_ragx.bootstrap`.

## Run & test
```bash
cd ragx
uv run pytest -q tests/integration/test_admin_ui.py
```

## Conventions
The account is created once from `RAG_ADMIN_EMAIL` / `RAG_ADMIN_INITIAL_PASSWORD` (≥ 12 characters); later changes to those variables do nothing. Sessions are random tokens stored as SHA-256; expired rows are removed at the next login. Unknown emails are verified against a decoy hash so both failure paths take the same time. The limiter counts failures per hashed email and per IP. Without Redis, `check` and `record_failure` fail closed (`PROVIDER_UNAVAILABLE`); `reset` after a correct password only logs a warning, because the counters expire on their own.

## Common failures
- Locked out (`RATE_LIMITED`) → too many failures → wait `RAG_UI_LOGIN_WINDOW_SECONDS`.
- Forgot the password → the account exists already → `DELETE FROM admin_users`, set a new `RAG_ADMIN_INITIAL_PASSWORD`, rerun `ragx bootstrap`.
- `admin account not configured` in bootstrap logs → `RAG_ADMIN_EMAIL` or `RAG_ADMIN_INITIAL_PASSWORD` is unset.
