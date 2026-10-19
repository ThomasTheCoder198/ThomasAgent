# features/agents

## Purpose

Agents list, create and delete, and per-Agent settings: name and description, instructions, the one attached Knowledge Base,
the read-only KB tools (all automatic until M3a brings per-tool policies), model overrides per role. Everything is
scoped to the current org (`/api/v1/orgs/{orgId}/agents`). Edits stay in a local draft until one PATCH saves exactly
what changed.

A save reports its outcome by toast (`useNotify`): "Saved" on success, the localized catalog message (plus the
request ID for 5xx) on failure. The name field keeps its inline validation error.

Creating an Agent asks for a name, an optional description and one of the org's KBs (the first by default); a taken
name (`CONFLICT` with `details.fields` naming `name`) stays on the field until edited, never in a toast. A failed KB
list shows the route's 404 state when missing, a retryable service error for 5xx, or a catalog refusal with Retry for
other errors on `/agents`; Create appears only after the KB list loads, and only a successfully loaded empty list opens
the no-KB dialog. Deleting an Agent also deletes its conversations (the confirmation says so); the KB and its files stay in ragx.
An Agent whose KB ragx no longer has (`kbMissing`) shows a destructive "KB deleted" badge in the list and the header.

## Entry points

- `AgentList.tsx` — server-rendered ruled rows for `/agents`: the whole row is a link; name over description, then `AgentMeta` (beside the name from md, stacked below it on phones). Empty list shows the shell `EmptyState` inviting the first Agent.
- `api.ts` — `createAgent(orgId, input, locale)` and `deleteAgent(orgId, id, locale)`; client-side mutations through `apiFetch` (CSRF headers).
- `components/NewAgentDialog.tsx` — `NewAgentDialog` (name, description, KB Select; no KB in the org → a note linking to the KB page and a disabled Create) and `NewAgentButton`, the `/agents` header action; success opens the new Agent's page.
- `components/DeleteAgentSection.tsx` — the Overview tab's last row: destructive button plus `ConfirmDelete`; on success back to `/agents`.
- `AgentListSkeleton.tsx`, `AgentSettingsSkeleton.tsx` — Skeletons shaped like the list rows and the detail page, used by `app/(workspace)/agents/loading.tsx` and `[id]/loading.tsx`.
- `AgentSettings.tsx` — client page for `/agents/[id]`: `AgentHeader`, sticky shadcn underline `Tabs` (scrolls sideways on phones), tab bodies inside a `fieldset` that is disabled while saving, `SaveBar`.
- `components/AgentHeader.tsx`, `components/AgentMeta.tsx` — back link and Newsreader page title, then the meta row (KB badge, line dots + tool count, model in mono) that the list rows share. `kbMissing` turns the KB badge destructive and reads "KB đã bị xoá".
- `components/SaveBar.tsx` — sticky bar (card, outline, lift) that rises only while the draft is dirty; Save is the primary Button with `pending`. Its visible marker reserves desktop toast space for both Agent and Settings pages.
- `draft.ts` — `fromAgent`, `diffDraft` (smallest `AgentPatch`; trims the name, empties whitespace-only descriptions), `isDirty`, `draftErrors` (required and too-long, mirroring core's name/description/instructions caps), `canSave`.
- `useAgentEditor.ts` — draft state and `persist()`. While a save is in flight the draft is frozen (`update`/`discard` are ignored), so the server's answer never overwrites typing. A failed save keeps the draft.
- `model-choice.ts` — maps the Select's `__inherit__` sentinel (Radix forbids an empty-string value) to "no override" at the boundary.
- `components/KnowledgeTab.tsx` — a Select; load failure shown as an error; a binding to a KB missing from the list stays selectable as an "unknown Knowledge Base" item with a warning.
- `components/ModelsTab.tsx` — one Select per role (the inherited model is the org's effective role); `ToolsTab.tsx` — read-only list of the four KB tools, each "Tự động" (ruling C2-3).
- `tab-ids.ts` — tab ids and `TAB_QUERY_KEY` shared by the server page (`?tab=`) and the client.
- `contract.ts` — org-scoped `AGENT_PATHS`, `agentRoles`, the field-length caps and `AgentRow`. The `Agent` shape is core's generated one (`lib/core/types.ts`).
- `server.ts` — `listAgents(orgId)` and `getAgentPage(orgId, id)`. Everything but the Agent itself comes back as `Loaded<T>` (the org's KBs through the ragx app proxy, models, the org's model roles); a failure renders as an error state, never as an empty list. A missing Agent (`AGENT_NOT_FOUND`) is `null`, which the page turns into not-found.

## Dependencies

- Uses: `components/ui` (Tabs, Select, SegmentedChoice, SettingsSection, Field, Input, Textarea, InlineAlert, Badge, Skeleton, Button), `components/metro` (LineBadge, LineDots), `components/lineIcons`, `features/shell` (PageHeader, EmptyState, MenuButton), `lib/useNotify`, `features/registry/types`, `features/knowledge-bases` (`listKnowledgeBases`, `ConfirmDelete`), `features/orgs/contract` (`orgHome`), `lib/api` (`serverFetchResult`, `Loaded`), `lib/core/types`, `lib/ragx/types`.
- Used by: `app/(workspace)/agents/*`.

## Run & test

```bash
cd frontend
npx vitest run features/agents
npm run e2e -- e2e/agents.spec.ts
```

## Conventions

- **Tenancy (AGENTS rule 11):** core must validate that the submitted `kbId` and every model id in `modelOverrides` belong to the caller's tenant, taken from context. The client's values are never trusted; the UI only offers what the tenant's lists contain. The fake core mimics this (a KB the org does not have → `AGENT_KB_INVALID`).
- Exactly one KB per Agent; embedding and rerank roles belong to the KB, so Agents override only `chat.default`, `chat.fast`, `vision`.
- Tool policies are not editable in M2 (core's `Agent` has none); they return with M3a.
- Creating needs a KB: with none in the org the dialog explains it and Create stays disabled. Core still checks `kbId` (`AGENT_KB_INVALID` 422).
- An empty or whitespace-only name never reaches the network: Save is disabled and the field shows an inline error.

## Common failures

- Save bar never appears → the draft equals the stored Agent after normalising (trimmed name, empty description).
- "Không tải được danh sách …" on the Knowledge or Models tab → core returned 404/5xx for that list; it is reported, not hidden.
