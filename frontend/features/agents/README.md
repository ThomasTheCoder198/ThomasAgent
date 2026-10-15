# features/agents

## Purpose

Agents list and per-Agent settings: name and description, instructions, the one attached Knowledge Base,
per-tool policy (auto / ask / off) grouped by tool source, model overrides per role, context files. Edits stay in
a local draft until one PATCH saves exactly what changed.

A save reports its outcome by toast (`useNotify`): "Saved" on success, the localized catalog message (plus the
request ID for 5xx) on failure. The name field keeps its inline validation error.

## Entry points

- `AgentList.tsx` — server-rendered ruled rows for `/agents`: the whole row is a link; name over description, then `AgentMeta` (beside the name from md, stacked below it on phones). Empty list shows the shell `EmptyState`.
- `AgentListSkeleton.tsx`, `AgentSettingsSkeleton.tsx` — Skeletons shaped like the list rows and the detail page, used by `app/(workspace)/agents/loading.tsx` and `[id]/loading.tsx`.
- `AgentSettings.tsx` — client page for `/agents/[id]`: `AgentHeader`, sticky shadcn underline `Tabs` (scrolls sideways on phones), tab bodies inside a `fieldset` that is disabled while saving, `SaveBar`.
- `components/AgentHeader.tsx`, `components/AgentMeta.tsx` — back link and Newsreader page title, then the meta row (KB badge, line dots + tool count, model in mono) that the list rows share.
- `components/SaveBar.tsx` — sticky bar (card, outline, lift) that rises only while the draft is dirty; Save is the primary Button with `pending`.
- `draft.ts` — `fromDetail`, `diffDraft` (smallest `AgentPatch`; trims the name, empties whitespace-only descriptions), `isDirty`, `draftErrors`, `canSave`.
- `useAgentEditor.ts` — draft state and `persist()`. While a save is in flight the draft is frozen (`update`/`discard` are ignored), so the server's answer never overwrites typing. A failed save keeps the draft.
- `model-choice.ts` — maps the Select's `__inherit__` sentinel (Radix forbids an empty-string value) to "no override" at the boundary.
- `components/KnowledgeTab.tsx` — a Select; load failure shown as an error; a binding to a KB missing from the list stays selectable as an "unknown Knowledge Base" item with a warning.
- `components/ModelsTab.tsx` — one Select per role; `ToolsTab.tsx` — per-tool policy as a `SegmentedChoice`, grouped by tool source.
- `tab-ids.ts` — tab ids and `TAB_QUERY_KEY` shared by the server page (`?tab=`) and the client.
- `contract.ts` — PROVISIONAL M2 shapes and endpoints (`/api/v1/agents`, `/agents/{id}`, `/knowledge-bases`), shared with `mock-core/`. Not in `contracts/openapi` yet.
- `server.ts` — loaders. Only the Agent **list** may fall back to empty on 404; knowledge bases, models and roles come back as `Loaded<T>` and a failure renders as an error state, never as an empty list.

## Dependencies

- Uses: `components/ui` (Tabs, Select, SegmentedChoice, SettingsSection, Field, Input, Textarea, InlineAlert, Badge, Skeleton, Button), `components/metro` (LineBadge, LineDots), `components/lineIcons`, `features/shell` (PageHeader, EmptyState, MenuButton), `lib/useNotify`, `features/registry/types`, `lib/api` (`serverFetchResult`, `Loaded`), `lib/format` (`formatBytes`).
- Used by: `app/(workspace)/agents/*`.

## Run & test

```bash
cd frontend
npx vitest run features/agents
npm run e2e -- e2e/agents.spec.ts
```

## Conventions

- **Tenancy (AGENTS rule 11):** core must validate that the submitted `kbId` and every model id in `modelOverrides` belong to the caller's tenant, taken from context. The client's values are never trusted; the UI only offers what the tenant's lists contain. The fake core mimics this (another org's KB → `FORBIDDEN`, unknown ids → `VALIDATION_FAILED`).
- Exactly one KB per Agent; embedding and rerank roles belong to the KB, so Agents override only `chat.default`, `chat.fast`, `vision`.
- Risky tools default to `ask`; the policy control never hides that a tool is risky.
- An empty or whitespace-only name never reaches the network: Save is disabled and the field shows an inline error.

## Common failures

- Save bar never appears → the draft equals the stored Agent after normalising (trimmed name, empty description).
- "Không tải được danh sách …" on the Knowledge or Models tab → core returned 404/5xx for that list; it is reported, not hidden.
