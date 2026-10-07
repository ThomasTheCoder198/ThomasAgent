# app/(workspace)

## Purpose

Authenticated workspace routes share the sidebar and preserve it during page loading and recoverable page errors.

## Entry points

- `layout.tsx` — session, recent conversations, and sidebar.
- `loading.tsx`, `error.tsx`, `not-found.tsx` — content-area states within the shell.
- `chat/`, `agents/`, `knowledge-bases/`, `tools/`, `usage/` — workspace pages.

## Dependencies

- Uses `features/shell`, `features/auth`, `features/chat`, and `features/agents`.

## Run & test

Run `npm run lint`, `npm run typecheck`, and `npm run e2e` from `frontend/`.
