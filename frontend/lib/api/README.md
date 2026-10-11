# lib/api

## Purpose

The response-aware HTTP client for core: unwraps `{ data }`, turns `{ error }` into a typed `ApiError` with a
catalog code, maps network failures and non-JSON bodies to `CLIENT_NETWORK_ERROR`, and attaches
`X-CSRF-Token` from the `thomas_csrf` cookie on unsafe methods.

Every call sends a fresh `X-Request-Id` (`request-id.ts`, values copied from `contracts/request-id.json` and checked by `request-id.test.ts`); `ApiError.requestId` comes from `error.requestId` or the response header, and `describeApiError` appends it to the user-facing message.

## Entry points

- `client.ts` — `apiFetch` (browser), `apiFetchPage` (list endpoints: resolves `{ data, page: { cursor, limit } }` from `meta.page`), `parseResponse`, `mutationHeaders` (also used by the chat transport), `csrfHeaders`, `networkError`, `ApiError`. A **204 No Content** answer (ragx deletes, passed through by core's app proxy) resolves `undefined` instead of being reported as a network error, so type such calls `apiFetch<void>`.
- `server.ts` — `serverFetch` and `serverFetchPage` (same shape as `apiFetchPage`) for server components, sharing one request builder; calls `WEB_CORE_URL` directly and forwards the browser's cookies. `serverFetchOptional` (404 → fallback) is only for endpoints core may not serve yet where "empty" is a true answer (e.g. the Agent list); `serverFetchResult` returns a `Loaded<T>` so any other failure is rendered as an error state instead of an empty list.
- `request-id.ts` — `newRequestId` and the shared header/pattern constants.
- `loaded.ts` — the `Loaded<T>` type and `loadedOr`, shared by server loaders and client components.

## Dependencies

- Uses: `lib/errors/codes.gen.ts` (generated from `contracts/errors.yaml`), `lib/env.ts`.
- Used by: `features/*`.

## Run & test

```bash
npx vitest run lib/api
```

## Common failures

- `CLIENT_NETWORK_ERROR` for every call → core down or `WEB_CORE_URL` wrong → use `bun run dev:mock` for fake core, or check that real core is running at `WEB_CORE_URL` (default `http://localhost:8080`) with `bun run dev`.
