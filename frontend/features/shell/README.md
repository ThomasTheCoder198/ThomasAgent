# features/shell

## Purpose

The workspace frame: the enamel-black signage sidebar (brand, new chat, ⌘K placeholder, nav with line dots,
recents, owner plate with the settings menu), its desktop icon rail, the mobile drawer, page headers and the
one-station empty state.

## Entry points

- `Sidebar.tsx` — presentational; folds to a 64px rail through the `group/shell` `data-collapsed` variant.
- `ShellFrame.tsx` + `useShellState.ts` — drawer below `md`; rail toggle on desktop (button or ⌘/Ctrl+B), persisted in the `thomas_sidebar` cookie (`sidebar-state.ts`) so the server renders the right width.
- `shell-menu.ts` — context for `MenuButton` / `CollapseButton`; `pageOwnsHeader()` lists routes whose header carries the mobile menu.
- `NavLink.tsx` — `aria-current` from the live pathname (layouts persist across navigations).
- `PageHeader.tsx`, `UserMenu.tsx`, `EmptyState.tsx`.

## Dependencies

- Uses: `components/metro`, `components/ui`, `features/preferences`, `features/auth`.
- Used by: `app/(workspace)/layout.tsx` and workspace pages.

## Run & test

```bash
npx vitest run features/shell
npm run e2e -- e2e/agents.spec.ts   # includes the rail collapse test
```

## Common failures

- Long recent titles wrap → each title is `truncate` with a `title` tooltip; keep the grid `1fr auto`.
- Popover clipped in the rail → never add `overflow-hidden` to the sidebar `aside`; labels already turn `sr-only`.
