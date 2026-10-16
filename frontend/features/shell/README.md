# features/shell

## Purpose

The workspace frame (Editorial Trust, `DESIGN.md` "Layout & Spacing" and "Navigation"): the sidebar (brand, org
switcher, new chat, search placeholder, nav, recents, owner plate with the settings menu), its desktop icon
rail, the mobile drawer, page headers, empty states and the route status pages.

## Entry points

- `Sidebar.tsx` — presentational; a 256px (`w-64`) surface with a right border, folding to a 64px icon rail through the `group/shell` `data-collapsed` variant. Rows are 40px (44px below md); the current page is a card fill with a 1px outline, a violet icon and a 3px violet bar that slides between rows (`NavGroup` scopes it per sidebar). New chat is the primary `Button` (a link). The `org` slot renders the org switcher (`features/orgs`) under the brand.
- `ShellFrame.tsx` + `useSidebarRail.ts` — below `md` the same sidebar opens in a left shadcn `Sheet` (named by `shell.nav`, closes on navigation and on any link inside it); pages without their own header get a 56px top bar with the menu button and the brand. On desktop the rail toggle (button or Cmd/Ctrl+B) is persisted in the `thomas_sidebar` cookie (`sidebar-state.ts`) so the server renders the right width.
- `shell-menu.ts` — context for `MenuButton` / `CollapseButton`; `pageOwnsHeader()` lists routes whose header carries the mobile menu button (chat, agents, orgs).
- `MenuButton.tsx` — opens the drawer; renders nothing outside the shell.
- `UserMenu.tsx` — shadcn `DropdownMenu` on the sidebar's settings button: theme and language radio items (`features/preferences`) and sign out. A failed sign-out toasts and stays; an expired session counts as signed out.
- `NavLink.tsx` — `aria-current` from the live pathname (layouts persist across navigations).
- `PageHeader.tsx` — Newsreader page title, muted description, actions (under the title below sm), `max-w-5xl` with 16/24/32px gutters.
- `EmptyState.tsx` — centred: a 32px icon in a 48px circle (`line` tints it, `icon` overrides the line's glyph), a title, a muted body (max 56ch), optional action.
- `RouteStatus.tsx` — route boundaries: `loading` is a Skeleton shaped like a page (announced through a `role="status"` node), `error` is a retryable service error with the `requestId` in mono, `notFound` is the 404 with a way back. Root and workspace variants share one copy.

## Dependencies

- Uses: `components/metro`, `components/ui` (Sheet, DropdownMenu, Button, Skeleton), `features/preferences`, `features/auth`, `lib/useNotify`, `hooks/useMediaQuery`.
- Used by: `app/(workspace)/layout.tsx`, workspace pages and the KB / chat / agent headers (`MenuButton`, `pageOwnsHeader`).

## Run & test

```bash
npx vitest run features/shell
npm run e2e -- e2e/agents.spec.ts   # includes the rail collapse test
```

## Common failures

- Long recent titles wrap -> each title is `truncate` with a `title` tooltip; keep the grid `1fr auto`.
- A menu is clipped in the rail -> never add `overflow-hidden` to the sidebar `aside`; labels already turn `sr-only`.
- The drawer shows the full sidebar even when the rail is folded -> expected: the Sheet portals out of `group/shell`, so the rail variants do not apply there.
- Two menu buttons on a phone -> a page header with its own `MenuButton` must be listed in `pageOwnsHeader()`, otherwise the shell bar adds one.
