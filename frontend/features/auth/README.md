# features/auth

## Purpose

Owner sign-in and the server-side session check. Cookies (`thomas_session`, `thomas_csrf`) are set by core;
this module never stores credentials or tokens.

## Entry points

- `LoginForm.tsx` — posts to `POST /api/v1/auth/login` and follows `next`.
- `session.ts` — `requireUser(nextPath)` for server layouts; redirects to `/login?next=…` on `UNAUTHENTICATED` / `AUTH_SESSION_EXPIRED`.
- `paths.ts` — auth endpoints, routes, `safeNext` (same-origin only), cookie name used by `proxy.ts`.

## Dependencies

- Uses: `lib/api/client`, `lib/api/server`, core auth endpoints (M0.2).
- Used by: `app/login`, `app/(workspace)/layout.tsx`, `proxy.ts`, `features/shell/UserMenu`.

## Run & test

```bash
npx vitest run features/auth
npm run e2e -- e2e/login.spec.ts
```

## Common failures

- Redirect loop to `/login` → core is unreachable or the cookie is not first-party → keep `/api/v1` on the same origin via the rewrite.
