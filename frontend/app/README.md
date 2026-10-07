# app

## Purpose

Next.js routes and route boundaries for the ThomasAgent frontend. The root layout owns locale and theme providers.

## Entry points

- `layout.tsx` — root document and providers.
- `loading.tsx`, `error.tsx`, `not-found.tsx` — full-viewport states while a layout loads, when it fails, and for unknown URLs.
- `(workspace)/` — authenticated workspace shell and its route states.

## Dependencies

- Uses `features/shell`, `features/auth`, and `messages/`.

## Run & test

Run `npm run lint`, `npm run typecheck`, and `npm run e2e` from `frontend/`.
