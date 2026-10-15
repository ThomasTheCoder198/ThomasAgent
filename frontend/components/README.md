# components

## Purpose

Presentational building blocks with no data fetching. `metro/` is the world's vocabulary; `ui/` holds restyled
controls.

## Entry points

- `metro/lines.ts` — the single map from a line (`kb | mcp | cmp | app | model`) to its classes; use it instead of writing colours.
- `metro/lineIcons.ts` — one glyph per tool line (KB library, MCP plug, Composio workflow, Apps window), shared by the run line, approval signs and Agent settings.
- `metro/LineBadge.tsx` — a line's solid name plate; `metro/LineDots.tsx`, `metro/BrandMark.tsx`, `metro/RouteStrip.tsx`.
- `ui/Button.tsx` — `Button` (with `pending` departing bar) and `IconButton` (mandatory `label`).
- `ui/TextField.tsx` — labelled input with `error` (sets `aria-invalid`, links the message).
- `ui/TextArea.tsx` — growing multiline field with an optional footer counter.
- `ui/Segmented.tsx` — radio group in `sign` or `ground` tone.
- `ui/Tabs.tsx` — WAI-ARIA tabs: roving tabindex, ←/→ (wrapping), Home/End; pair with `tabPanelId()` and give the panel `tabIndex={0}`.
- `ui/SettingsSection.tsx` — one ruled settings row (title + hint left, control right).
- `ui/InlineAlert.tsx` — in-place error with the server's message.
- `ui/Toast.tsx` — `ToastProvider` (mounted once in `app/layout.tsx`) and `useToast().show({ tone, title, detail })`: lifted sign notices, bottom right (bottom, full width on phones). Errors go into an always-present `role=alert` region and stay until dismissed; info goes into `role=status` and leaves after `INFO_TOAST_MS`; at most `MAX_TOASTS`. Both regions set `aria-live` (so a modal's aria-hidden spares them) and `aria-atomic=false` (a new toast is read alone). The title is plain text and names the dismiss button; dismissing the focused toast moves focus to the next one or the region; info timers are cleared on dismiss and unmount. `isInToastRegion` lets `DialogContent` ignore clicks on a toast instead of closing. Custom rather than Radix Toast, whose announcements are always `role=status`.
- `ui/Dialog.tsx`, `ui/DropdownMenu.tsx`, `ui/Switch.tsx` — wrappers over `radix-ui` 1.6.7 restyled to Metro (lifted paper dialog on the scrim; menus that open from the sign wear the sign). Never build these controls by hand: Radix owns focus trapping, roving focus, Escape and the switch semantics.
- `ui/useDismiss.ts` — Escape / outside-press closing.

## Dependencies

- Uses: `lib/cn`, lucide-react.
- Used by: every feature.

## Run & test

```bash
cd frontend
npx vitest run components
```

## Conventions

New controls take an accessible name. Colours come from tokens in `app/globals.css`. Motion uses the shared
keyframes there (`animate-rise | fade | pop | stem | draw | draw-x | depart | hit`) and the `ease-out-expo`
curve; every control presses to `scale(0.97)`. `prefers-reduced-motion` collapses durations and delays.
