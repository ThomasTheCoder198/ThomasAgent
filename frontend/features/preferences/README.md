# features/preferences

## Purpose

Theme (`light | dark | system`, via next-themes on `data-theme`) and language (`vi | en`, cookie `NEXT_LOCALE`).

## Entry points

- `ThemeSwitcher.tsx`, `LocaleSwitcher.tsx` — both drawn with `components/ui/Segmented`.
- `actions.ts` — `setLocale` server action.

## Dependencies

- Uses: `next-themes`, `next-intl`, `i18n/config.ts`.
- Used by: `features/shell/UserMenu`.

## Run & test

```bash
npx vitest run features/preferences
```
