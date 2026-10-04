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
