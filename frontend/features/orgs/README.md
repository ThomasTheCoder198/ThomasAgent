# features/orgs

## Purpose

The operator's organizations as the web app shows them: the org plate on the sidebar sign, the org menu, and the
create / settings dialogs. Core owns the data; ragx is provisioned by core, never by the browser.

## Entry points

- `contract.ts` — `Organization`, `RagxStatus`, `ORG_PATHS`, `CURRENT_ORG_COOKIE` (`thomas_org`), `ORG_NAME_MAX_LENGTH`, `orgHome(id)` and `validateOrgName(raw)` (mirrors core: trimmed, 1-200 characters, no control characters).
- `current.ts` — `resolveCurrentOrg(orgs, pathOrgId, cookieOrgId)` and `orgIdFromPath(path)`.
- `server.ts` — `listOrganizations()` and `getOrganization(id)` for server components (`server-only`).
- `api.ts` — `createOrganization`, `updateOrganization`, `retryOrganization` (each resolves to the org's state).
- `OrgPlate.tsx` — the org switcher (a `DropdownMenu`) and its menu; `OrgDialog.tsx` — `mode="create" | "edit"`; a save always ends with `router.refresh()` (create also pushes to the new org), because the workspace layout that lists the orgs stays mounted across navigations; `useLiveOrgId.ts` — the one hook the plate and the Knowledge Base nav link share (URL org, else last seen, else server org); `OrgStatusDot.tsx`.

## Current-org rule

Path (`/orgs/{id}/...`) -> the `thomas_org` cookie (set by `proxy.ts` whenever a page under `/orgs/{id}` is opened) -> the
first org. An id that is not in the operator's list is ignored. This is memory for the UI, never authority: core
re-checks the org on every request.

## The plate and the dialogs

- A switcher row on the sidebar under the brand: an initial roundel, the org's name, a status dot and a chevron. On the folded icon rail it shrinks to the roundel.
- The menu lists every org with its status, then _Org settings_ (current org) and _New organization_. Core's UI never links to the ragx admin UI (`/ui`, model settings): it is a separate surface (owner decision P14, reversed 2026-10-14). The KB screens under `features/knowledge-bases` are ragx's app surface inside core's shell and reach ragx only through the app proxy.
- Status dots: **ready** = solid `success`, **pending** = hollow ring, **failed** = `destructive`.
- The settings dialog's **Save** stays disabled until the draft differs from the saved org (trimmed name, external parsing); reverting the edit disables it again, so an unchanged form never sends a PATCH (owner, 2026-10-14). Create mode is always submittable and validates on submit.

## Outcomes

Every mutation reports through `useNotify`: a success toast for a create or save (`orgs.created`, `orgs.saved`), followed by a problem toast (`orgs.errors.notReady`; the ragx error code stays in the dialog and the org's station) when the org is not ready and synced; a Retry gets its success toast (`orgs.retried`) only when the org comes back ready; a failed call toasts the catalog message for its code (request ID for 5xx). A taken name stays a field error, with no toast.

## Synchronous provisioning and Retry

Core provisions into ragx during create / provision, so the answer already carries the real state. The settings dialog
shows **Retry** when `ragxStatus !== "ready"` or `!settingsSynced`. A `pending` org is therefore stuck (a core
process died mid-call), and Retry is its way out. `POST /api/v1/orgs/{id}/provision` answers 200 with the real
state; the dialog shows it at once and calls `router.refresh()`. The UI never keys on the code `RAGX_UNAVAILABLE`.

## Dependencies

- Uses: `components/ui` (Dialog, DropdownMenu, Field / SwitchField, Input, Button), `lib/api`, `lib/useNotify`.
- Used by: `app/(workspace)/layout.tsx`, `proxy.ts`, the Knowledge Base surface (Task 7).

## Run & test

```bash
cd frontend
npx vitest run features/orgs proxy.test.ts
npm run dev:mock   # Công ty Demo (ready), Beta (failed), Gamma (stuck pending); "Acme !fail" creates a failed org
```

## Common failures

- The plate says "Organizations unavailable" -> core's `GET /api/v1/orgs` failed; the rest of the shell still renders.
