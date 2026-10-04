# components

## Purpose

Presentational building blocks with no data fetching. `metro/` is the world's vocabulary (line colours,
`LineDots`, `BrandMark`, `RouteStrip`); `ui/` holds restyled controls (`Button`, `IconButton`, `TextField`,
`Segmented`, `useDismiss`).

## Entry points

- `metro/lines.ts` — the single map from a line (`kb | mcp | cmp | app | model`) to its classes; use it instead of writing colours.

## Dependencies

- Uses: `lib/cn`, lucide-react.
- Used by: every feature.

## Conventions

New controls take an accessible name (`IconButton` requires `label`). Colours come from tokens in `app/globals.css`.
