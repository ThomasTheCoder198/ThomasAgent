# Admin UI

## Purpose
Server-rendered model-config UI at `/ui`: Login, Providers, Models, Roles. Templates call `RegistryService` and `AdminService` directly, never ragx's own HTTP API. Direction contract: `.impeccable/surfaces/ragx-ui.md`; design decisions: the "Ragx admin UI" section of `DESIGN.md`.

## Entry points
- `install_ui` — called by `create_app`; mounts `/ui/static`, adds the router and the `/ui` error pages.
- `routes.py` — pages and post/redirect/get actions; `session.py` — session and CSRF dependencies; `i18n.py` — VI/EN catalog.
- `templates/`, `static/ragx.css`, `static/app.js` (theme, drawer, dialogs, pending bar), `messages/{vi,en}.json`.

## Dependencies
- Uses: Jinja2, htmx 2.0.11 (vendored), Lexend and Geist Mono from `@fontsource-variable` 5.3.0 (vendored, OFL), `RegistryService`, `AdminService`.
- Used by: `thomas_ragx.api.app`.

## Run & test
```bash
cd ragx
uv run pytest -q tests/unit/test_ui_tokens.py tests/unit/test_ui_messages.py tests/integration/test_admin_ui.py
```
Then rebuild with `task up:ragx` and check `/ui` in a browser at 390 px and 1280 px, light and dark, VI and EN.

## Conventions
- Tokens are copied verbatim from `frontend/app/globals.css` and checked by `test_ui_tokens.py`; edit them in the frontend first. UI-only values live outside `:root` (for example `--target-size: 44px` on `html`, because the 15px root makes `2.75rem` only 41px).
- CSP allows only `'self'`: no inline script or style; `<meta name="htmx-config">` turns off htmx's injected indicator style.
- Every unsafe request carries the CSRF token (hidden field, or `X-CSRF-Token` from `hx-headers`).
- Post/redirect/get with `?notice=` or `?error=<catalog code>&rid=<failing request id>`, so the page shows the id that is in the logs. 5xx codes are logged once and shown as `INTERNAL_ERROR`.
- Native controls only. Delete confirms in a native `<dialog>`; edit opens a `<details>` disclosure; Test swaps an inline result under its row; pending is the departing-train bar.
- A message key used in a template must exist in both catalogs (`test_ui_messages.py`).
- Vendored file SHA-256:
  - `vendor/htmx.min.js` d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717
  - `vendor/htmx.LICENSE` d3d2456f76414f2456104660ebd65aff1c04cd7966b942bdabd63f3cdb316a38
  - `fonts/lexend-latin-wght-normal.woff2` b7e7538f59bff1812841d7627f8e49e70fe86438f9f7b397a028c0d05301c874
  - `fonts/lexend-latin-ext-wght-normal.woff2` b5d815d42066da25e5894bdfe47a415e1f79ef436486e3f843b475806d2e613c
  - `fonts/lexend-vietnamese-wght-normal.woff2` aab0b318aab11e9f0bcd21ff9d2b0ff6ad7a3f061a071b53af670d4749d2cba2
  - `fonts/geist-mono-latin-wght-normal.woff2` 684ad5b531f81d43c1e8c7038262d5db7cdc1f68006e04d6c7769efa8d33c8cc
  - `fonts/geist-mono-latin-ext-wght-normal.woff2` 1a189eb997c3e2ece68373e387afaec9e8617424186c4b1ab3cff7c54ba6223b
  - `fonts/OFL-lexend.txt` cdfed8f91b0660b004befb744757918b7c7d40dc2201b6c6d0a86b2fab9408f8
  - `fonts/OFL-geist-mono.txt` cc815ed4fc045f0e991abb10395b7932bd028c6a067deb13316d6002105074e6

## Common failures
- 403 `AUTH_CSRF_INVALID` → stale page after a new login → reload.
- Unstyled page → static files missing from the wheel → check the image contains `thomas_ragx/ui/static`.
- A failed add clears the form → post/redirect/get does not carry field values back → re-enter them (known limitation).
