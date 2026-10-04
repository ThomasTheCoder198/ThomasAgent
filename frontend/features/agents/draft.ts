import { agentRoles, type AgentDetail, type AgentPatch, type AgentRole, type ToolPolicy } from "./contract";

/** What the settings tabs edit: a flat copy of the editable parts of an Agent. */
export type AgentDraft = {
  name: string;
  description: string;
  instructions: string;
  kbId: string;
  policies: Record<string, ToolPolicy>;
  overrides: Partial<Record<AgentRole, string>>;
};

export function fromDetail(agent: AgentDetail): AgentDraft {
  const policies = Object.fromEntries(agent.toolSources.flatMap((s) => s.tools.map((t) => [t.id, t.policy])));
  return {
    name: agent.name,
    description: agent.description,
    instructions: agent.instructions,
    kbId: agent.kbId,
    policies,
    overrides: { ...agent.modelOverrides },
  };
}

function changedPolicies(base: AgentDraft, draft: AgentDraft): Record<string, ToolPolicy> {
  return Object.fromEntries(
    Object.entries(draft.policies).filter(([id, policy]) => base.policies[id] !== policy),
  );
}

function changedOverrides(base: AgentDraft, draft: AgentDraft): AgentPatch["modelOverrides"] {
  const out: NonNullable<AgentPatch["modelOverrides"]> = {};
  for (const role of agentRoles) {
    const before = base.overrides[role];
    const after = draft.overrides[role];
    if (before !== after) out[role] = after ?? null;
  }
  return out;
}

/** The smallest PATCH that turns the stored Agent into the draft. */
export function diffDraft(agent: AgentDetail, draft: AgentDraft): AgentPatch {
  const base = fromDetail(agent);
  const patch: AgentPatch = {};
  if (draft.name.trim() !== base.name) patch.name = draft.name.trim();
  // A description of only spaces means "no description"; send it as empty, not as spaces.
  const description = draft.description.trim() === "" ? "" : draft.description;
  if (description !== base.description) patch.description = description;
  if (draft.instructions !== base.instructions) patch.instructions = draft.instructions;
  if (draft.kbId !== base.kbId) patch.kbId = draft.kbId;
  const policies = changedPolicies(base, draft);
  if (Object.keys(policies).length > 0) patch.toolPolicies = policies;
  const overrides = changedOverrides(base, draft);
  if (overrides && Object.keys(overrides).length > 0) patch.modelOverrides = overrides;
  return patch;
}

export function isDirty(agent: AgentDetail, draft: AgentDraft): boolean {
  return Object.keys(diffDraft(agent, draft)).length > 0;
}

export type DraftErrors = { name?: "required" };

/** Client-side checks that mirror core's validation, so an invalid draft never reaches the network. */
export function draftErrors(draft: AgentDraft): DraftErrors {
  return draft.name.trim() === "" ? { name: "required" } : {};
}

export function canSave(agent: AgentDetail, draft: AgentDraft): boolean {
  return isDirty(agent, draft) && Object.keys(draftErrors(draft)).length === 0;
}
