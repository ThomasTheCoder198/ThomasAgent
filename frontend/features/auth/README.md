# features/auth

## Purpose

Owner sign-in and the server-side session check. Cookies (`thomas_session`, `thomas_csrf`) are set by core;
this module never stores credentials or tokens.

Sign-in follows the form rules of `frontend/AGENTS.md`: missing or malformed fields (`credentials.ts`) are explained in place, and a rejected sign-in is an error toast with the catalog message for the code in the current language (the request ID shows for 5xx only).

## Entry points

- `LoginForm.tsx` — validates, posts to `POST /api/v1/auth/login` and follows `next`; failures go through `useNotify().error`. `app/login/page.tsx` keeps the Newsreader hero and theme/language menu above a centred OS window sign-in form.
- `credentials.ts` — `validateCredentials` (email required and shaped like an address, password required).
- `session.ts` — `requireUser(nextPath)` for server layouts; redirects to `/login?next=…` on `UNAUTHENTICATED` / `AUTH_SESSION_EXPIRED`.
- `paths.ts` — auth endpoints, routes, `safeNext` (same-origin only), cookie name used by `proxy.ts`.

## Dependencies

- Uses: `lib/api/client`, `lib/api/server`, `lib/useNotify`, `features/preferences`, core auth endpoints (M0.2).
- Used by: `app/login`, `app/(workspace)/layout.tsx`, `proxy.ts`, `features/shell/UserMenu`.

## Run & test

```bash
npx vitest run features/auth
npm run e2e -- e2e/login.spec.ts
```

## Common failures

- Redirect loop to `/login` → core is unreachable or the cookie is not first-party → keep `/api/v1` on the same origin via the rewrite.
