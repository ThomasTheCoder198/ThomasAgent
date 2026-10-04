# lib/api

## Purpose

The response-aware HTTP client for core: unwraps `{ data }`, turns `{ error }` into a typed `ApiError` with a
catalog code, maps network failures and non-JSON bodies to `CLIENT_NETWORK_ERROR`, and attaches
`X-CSRF-Token` from the `thomas_csrf` cookie on unsafe methods.

## Entry points

- `client.ts` — `apiFetch` (browser), `parseResponse`, `mutationHeaders` (also used by the chat transport), `ApiError`.
- `server.ts` — `serverFetch` for server components; calls `WEB_CORE_URL` directly and forwards the browser's cookies.

## Dependencies

- Uses: `lib/errors/codes.gen.ts` (generated from `contracts/errors.yaml`), `lib/env.ts`.
- Used by: `features/*`.

## Run & test

```bash
npx vitest run lib/api
```

## Common failures

- `CLIENT_NETWORK_ERROR` for every call → core down or `WEB_CORE_URL` wrong → check `npm run dev:mock` / compose.
