# features/shell

## Purpose

The workspace frame: the enamel-black signage sidebar (brand, new chat, ⌘K placeholder, nav with line dots,
recents, owner plate with the settings menu), the mobile drawer, and the one-station empty state.

## Entry points

- `Sidebar.tsx` — presentational; `activeHref` marks nav items and the open conversation.
- `ShellFrame.tsx` — desktop column vs. off-canvas drawer below `md` (Escape, scrim and navigation close it).
- `UserMenu.tsx` — theme, language, sign out.
- `EmptyState.tsx` — a station on its line for pages whose milestone has not landed.

## Dependencies

- Uses: `components/metro`, `components/ui`, `features/preferences`, `features/auth`.
- Used by: `app/(workspace)/layout.tsx` and the empty pages.

## Run & test

```bash
npx vitest run features/shell
```

## Common failures

- Long recent titles wrap → each title is `truncate` with a `title` tooltip; keep the grid `1fr auto`.
