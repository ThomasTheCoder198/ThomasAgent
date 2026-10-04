# features/agents

## Purpose

Agents list and per-Agent settings: name and description, instructions, the one attached Knowledge Base,
per-tool policy (auto / ask / off) grouped by tool source, model overrides per role, context files. Edits stay in
a local draft until one PATCH saves exactly what changed.

## Entry points

- `AgentList.tsx` — server-rendered departure-board table for `/agents`.
- `AgentSettings.tsx` — client page for `/agents/[id]`: `AgentSign` header, sticky `Tabs`, tab bodies inside a `fieldset` that is disabled while saving, `SaveBar`.
- `draft.ts` — `fromDetail`, `diffDraft` (smallest `AgentPatch`; trims the name, empties whitespace-only descriptions), `isDirty`, `draftErrors`, `canSave`.
- `useAgentEditor.ts` — draft state and `persist()`. While a save is in flight the draft is frozen (`update`/`discard` are ignored), so the server's answer never overwrites typing; the "Saved" flash timer is cleared on the next save and on unmount.
- `components/KnowledgeTab.tsx` — load failure shown as an error; a binding to a KB missing from the list shows as an "unknown Knowledge Base" row.
- `tab-ids.ts` — tab ids and `TAB_QUERY_KEY` shared by the server page (`?tab=`) and the client.
- `contract.ts` — PROVISIONAL M2 shapes and endpoints (`/api/v1/agents`, `/agents/{id}`, `/knowledge-bases`), shared with `mock-core/`. Not in `contracts/openapi` yet.
- `server.ts` — loaders. Only the Agent **list** may fall back to empty on 404; knowledge bases, models and roles come back as `Loaded<T>` and a failure renders as an error state, never as an empty list.

## Dependencies

- Uses: `components/ui` (Tabs, Segmented, SettingsSection, TextArea, TextField, InlineAlert, Button), `components/metro` (LineBadge, lineIcons), `features/registry/types`, `lib/api` (`serverFetchResult`, `Loaded`), `lib/format` (`formatBytes`).
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
