# frontend

## Purpose

The ThomasAgent web app (Next.js 16 App Router, React 19, Tailwind v4) in the Metro Wayfinding world: owner
login, the signage sidebar shell, and the chat surface with its run line, citations and evidence panel. It owns
no business data; everything comes from Go core through the same-origin `/api/v1/*` rewrite.

## Entry points

- `app/login/page.tsx` — owner sign-in.
- `app/(workspace)/layout.tsx` — session check (`requireUser`), sidebar, recents.
- `app/(workspace)/chat/page.tsx`, `chat/[id]/page.tsx` — chat surface (`features/chat`).
- `app/(workspace)/agents`, `agents/[id]` — Agents list and tabbed Agent settings (`features/agents`).
- `app/(workspace)/{knowledge-bases,tools,usage}` — designed empty states until their milestones.
- `app/(workspace)/template.tsx` — route-change fade; the sidebar rail state comes from the `thomas_sidebar` cookie.
- `proxy.ts` — redirects to `/login` without a `thomas_session` cookie; forwards the path as `x-pathname`.
- `app/globals.css` — Metro tokens for light (station white) and dark (platform night).

## Dependencies

- Uses: Go core (`WEB_CORE_URL`), `contracts/` (generated `lib/errors/codes.gen.ts`; registry shapes from `contracts/openapi/core.v1.yaml`).
- Used by: the owner in the browser / installed PWA.

## Run & test

```bash
npm install
bun run dev:mock    # fake core (mock-core/) on :8787 + next dev on :3000; sign in as owner@thomas.local / metro-wayfinding
bun run dev         # against real core: WEB_CORE_URL=http://localhost:8080 bun run dev
npm run lint && npm run typecheck && npm test && npm run build
CORE_AUTH_OWNER_EMAIL=… CORE_AUTH_OWNER_PASSWORD=… npm run e2e          # needs a running app + core (or fake core)
SCREENS=1 npx playwright test --grep @screens --project desktop        # review captures into ../.impeccable/review
```

The frontend runs on the host, not in compose. From the repo root, use `task web:dev`
or `task web:dev:mock`; `task lint:frontend` and `task test:frontend` also run from there.
Bun is only the script runner; install with npm and keep `package-lock.json` as the lockfile of record.

| Env            | Where                           | Meaning                                             |
| -------------- | ------------------------------- | --------------------------------------------------- |
| `WEB_CORE_URL` | server, build time for rewrites | Base URL of core (default `http://localhost:8080`). |
| `WEB_BASE_URL` | Playwright                      | App under test (default `http://localhost:3000`).   |
| `MOCK_CORE_*`  | `mock-core/`                    | See `mock-core/README.md`.                          |

## Conventions

- Data never flows through a mock inside app code: the UI always speaks HTTP to core. `mock-core/` is a separate fake core; switching to the real one is only `WEB_CORE_URL`.
- Layers: `components/` presentational primitives (`ui/`, `metro/`), `features/*` feature modules, `lib/` cross-cutting helpers. Feature components take view props; protocol knowledge lives in one mapper (`features/chat/run-view.ts`).
- Every string comes from `messages/{vi,en}.json` (parity test). Line colours come from `components/metro/lines.ts`; never hand-write a line colour.
- Chat endpoints are PROVISIONAL until M2 specifies them in OpenAPI (`features/chat/contract.ts`).
- S8 (SSE through the rewrite): with the plain `rewrites()`, Next gzips proxied responses for browsers and buffers the whole stream. Core's SSE response must send `Cache-Control: no-cache, no-transform` (the fake core does), or set `compress: false` here. Decide in S8.

## Common failures

- Labels in English in tests → the app follows `Accept-Language` → Playwright `locale: "vi-VN"` is set in `playwright.config.ts`.
- Chat arrives in one burst instead of streaming → gzip buffering through the rewrite → see S8 note above.
- ESLint crashes with `contextOrFilename.getFilename is not a function` → eslint-plugin-react version detection on ESLint 10 → keep `settings.react.version` pinned in `eslint.config.mjs`.
- `getByRole("alert")` matches two elements → Next's route announcer is also `role="alert"` → scope the locator.
