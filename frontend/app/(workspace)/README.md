# app/(workspace)

## Purpose

Authenticated workspace routes share the sidebar and preserve it during page loading and recoverable page errors.

## Entry points

- `layout.tsx` — session, recent conversations, and the sidebar inside `ShellFrame` (drawer below md).
- `loading.tsx`, `error.tsx`, `not-found.tsx` — content-area states within the shell (Skeleton, retryable service error, 404).
- `chat/`, `agents/`, `knowledge-bases/`, `tools/`, `usage/` — workspace pages.

## Dependencies

- Uses `features/shell`, `features/auth`, `features/chat`, and `features/agents`.

## Run & test

Run `npm run lint`, `npm run typecheck`, and `npm run e2e` from `frontend/`.
