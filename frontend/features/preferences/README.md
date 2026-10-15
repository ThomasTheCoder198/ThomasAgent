# features/preferences

## Purpose

Theme (`light | dark | system`, via next-themes on `data-theme`) and language (`vi | en`, cookie `NEXT_LOCALE`).

## Entry points

- `ThemeMenuGroup.tsx`, `LocaleMenuGroup.tsx` — radio items (`DropdownMenuRadioGroup`) for use inside any `DropdownMenuContent`; `light | dark | system` and `vi | en` stay the only choices.
- `PreferencesMenu.tsx` — a settings button with both groups, for pages without the shell menu (sign-in).
- `actions.ts` — `setLocale` server action.

## Dependencies

- Uses: `next-themes`, `next-intl`, `i18n/config.ts`.
- Used by: `features/shell/UserMenu`, `app/login`.

## Run & test

```bash
npx vitest run features/preferences
```
