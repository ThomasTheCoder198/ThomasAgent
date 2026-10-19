# features/settings

## Purpose

The Settings page (`/settings`): a model for each core role, for the platform default and for the org picked in
the sidebar switcher, plus the providers those models come from. Core owns the role rules and the data; this
module only edits them. Embedding and rerank belong to ragx and are not here.

## Entry points

- `app/(workspace)/settings/page.tsx` — loads `getSettingsData` (`server.ts`) and renders `components/SettingsModels.tsx`.
- `contract.ts` — `SETTINGS_ROUTE`, `ORG_ROLE_PATHS`, `rolePath(scope, role)` and the provider-kind rules mirrored from core.
- `role-fit.ts` — `SETTINGS_ROLES` and `fitsRole(model, role)`, the mirror of core's `RequirementsForRole`.
- `role-view.ts`, `scope-roles.ts` — pure rules: which face a role row shows, and what a role resolves to per scope.
- `useRoleDraft.ts` — the unsaved edits of one tab and `planSave` (draft → PUT / DELETE requests, in role order).
- `useSettingsPage.ts` — tab state, the discard prompt, save and sync.
- `api.ts` — `saveRoles`, `testModel`, provider create / update / delete / sync, `updateCapabilities`.
- `provider-form.ts` — the provider form's fields, validation and request bodies.
- `components/` — `SettingsModels` (page), `RoleList` and `RoleRow` (+ `RoleBody`, `RoleControls`, `RoleFacts`),
  `ModelCombobox` (Popover + Command), `ProvidersSection`, `ProviderTable`, `ProviderDialog`, `CapabilityDialog`.

## Dependencies

- Uses: core `GET/POST /providers`, `PATCH/DELETE /providers/{id}`, `POST /providers/{id}/sync`, `GET /models`,
  `PATCH /models/{id}`, `POST /models/{id}/test`, `GET/PUT/DELETE /model-roles[/{role}]`,
  `GET/PUT/DELETE /orgs/{orgId}/model-roles[/{role}]`; `features/registry/types.ts`; `features/orgs/server.ts`
  (`getCurrentOrg`); the agents `SaveBar`; `components/ui/Popover.tsx` and `Command.tsx` (`cmdk` 1.1.1).
- Used by: the workspace layout (nav entry "Cài đặt").

## Run & test

```bash
cd frontend
npm run test -- features/settings
npm run e2e -- e2e/settings.spec.ts --project=desktop   # needs `npm run dev:mock`
```

## Conventions

- The page edits one scope at a time; the draft belongs to the tab on screen and is dropped (after asking) when the tab changes.
- A successful save keeps the draft until the refreshed roles arrive, so rows never flash back to the old value.
- Core re-checks every role against the model's capabilities (`LLM_MODEL_CAPABILITY_MISMATCH`); `fitsRole` only orders the picker.
- The Test result ("Đã thử · N ms") lives in component state and disappears on reload (owner gate, answer 5).
- An org decision role with a fallback source names the fast chat model in that org, even when its fast chat role overrides the platform.
- Providers and models are platform-wide; only role assignments change with the scope.

## Common failures

- A failed Test keeps its catalog error toast visible; desktop toasts clear the shared SaveBar so Save remains reachable.
- A model appears under "Không đủ năng lực" → its capabilities lack what the role needs, or are unknown (use "Sửa" if core allows it).
- Test answers "Provider từ chối key hoặc cấu hình" → the provider has no valid key; add one from the provider's menu.
- The org tab says there is no organization → core lists none; create one on `/orgs`.
