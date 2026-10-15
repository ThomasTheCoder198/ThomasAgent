# components

## Purpose

Presentational building blocks with no data fetching. `ui/` holds the shadcn/ui controls (Radix underneath)
restyled to the Editorial Trust system in `DESIGN.md`;
the top level holds the brand wordmark and the tool-source components that feature code composes.

## Entry points

- `lines.ts` — the single map from a line (`kb | mcp | cmp | app | model`) to its classes; use it instead of writing colours.
- `lineIcons.ts` — one glyph per tool line (KB library, MCP plug, Composio workflow, Apps window), shared by the run line, approval signs and Agent settings.
- `BrandMark.tsx` — the wordmark: a 16px violet square and "ThomasAgent" in Newsreader (`labelClassName` lets the rail hide the text). `LineBadge.tsx` — a tool source's name pill in its line colour (`lineBg` + `onLine`, a contrast-checked pair). `LineDots.tsx` — small dots for the sources a conversation or nav item touches.
- `ui/Button.tsx` — `Button` (`variant`: `default | outline | secondary | ghost | destructive | link`; `size`: `sm | default | lg | hero | icon | icon-sm`; `asChild`; `pending`). Only `default` wears the sticker (ink outline + hard offset shadow, lifts on hover, sinks on press). `pending` sets `aria-busy`, disables the button and puts a `Loader2` spinner before the label (it replaces the glyph on icon sizes). Every size is at least 44px tall below md. An icon-only button needs `aria-label` (and `title`).
- `ui/Input.tsx`, `ui/Textarea.tsx`, `ui/Label.tsx`, `ui/Select.tsx` — fields on a card fill with the input stroke, 4px radius, 40px (44px below md).
- `ui/Field.tsx` — `Field` (label above, render-prop control, hint / error / mono footer below; wires `id`, `aria-describedby`, `aria-invalid`) and `SwitchField` (labelled on/off row with a hint).
- `ui/Dialog.tsx`, `ui/AlertDialog.tsx`, `ui/Sheet.tsx` — modal layers (14px radius, lift shadow). Pass `returnFocusRef` when the opener may unmount (a menu item); `useModalLayer` remembers the opener and falls back to `<main>`, and ignores presses on a toast. `DialogHeading` gives optional descriptions an sr-only fallback. `AlertDialog` is for confirmations that must be answered.
- `ui/DropdownMenu.tsx`, `ui/Tooltip.tsx` (`TooltipProvider` is mounted in the root layout), `ui/Tabs.tsx` (underline style), `ui/Switch.tsx`, `ui/Toggle.tsx`, `ui/ToggleGroup.tsx`, `ui/Badge.tsx`, `ui/Separator.tsx`, `ui/Skeleton.tsx`, `ui/ScrollArea.tsx` — shadcn components over `radix-ui` 1.6.7. Never build these controls by hand: Radix owns focus trapping, roving focus, Escape and the semantics.
- `ui/SegmentedChoice.tsx` — one-of-N choice on `ToggleGroup` (single); a press on the chosen segment does not clear it.
- `ui/SettingsSection.tsx` — one settings row (title + hint left, control right from `lg`), divided by hairlines.
- `ui/InlineAlert.tsx` — in-place error: destructive text and icon, no box.
- `ui/Sonner.tsx` — `Toaster` (mounted once in `app/layout.tsx`): bottom right on desktop, top centre below md, card fill, lift shadow, at most `MAX_VISIBLE_TOASTS`, close button. Raise toasts through `lib/notify.ts` / `lib/useNotify.ts`, never `toast()` directly. Sonner's live region is polite, so error notices wrap their text in a `role="alert"` node.
- `ui/useModalLayer.ts` — focus return and toast-aware outside-press handling shared by Dialog, AlertDialog and Sheet.

## Dependencies

- Uses: `lib/cn`, `lib/notify`, lucide-react, `class-variance-authority`, `sonner`, `tw-animate-css` (enter/exit utilities such as `animate-in`), next-themes, next-intl (close labels).
- Used by: every feature. `components.json` configures the shadcn CLI (aliases, `@/lib/cn` as the utils module). The CLI writes kebab-case files, which collide with PascalCase on NTFS: generate into a scratch directory and copy deliberately.

## Run & test

```bash
cd frontend
npx vitest run components lib
```

## Conventions

New controls take an accessible name. Colours come from tokens in `app/globals.css` (shadcn names: `bg-background`, `text-muted-foreground`, `border-border`, ...). Shadows: `shadow-sticker` only on the primary button and the composer; `shadow-lift` on floating layers. Tailwind inlines shadow values, so dark-theme shadows are driven by `--lift-ink`, not by redefining `--shadow-lift`. Motion uses the shared keyframes there (`animate-rise | fade | pop | stem | hit`, plus `caret`), `tw-animate-css` for overlays, and the `ease-out-expo` curve. `prefers-reduced-motion` collapses durations and delays.
