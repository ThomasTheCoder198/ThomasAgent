import type { Agent, AgentPatch } from "@/lib/core/types";

import {
  AGENT_DESCRIPTION_MAX,
  AGENT_INSTRUCTIONS_MAX,
  AGENT_NAME_MAX,
  agentRoles,
  type AgentRole,
} from "./contract";

/** What the settings tabs edit: a flat copy of the editable parts of an Agent. */
export type AgentDraft = {
  name: string;
  description: string;
  instructions: string;
  kbId: string;
  overrides: Partial<Record<AgentRole, string>>;
};

function overridesOf(agent: Agent): Partial<Record<AgentRole, string>> {
  const stored = (agent.modelOverrides ?? {}) as Record<string, string | undefined>;
  const out: Partial<Record<AgentRole, string>> = {};
  for (const role of agentRoles) {
    const id = stored[role];
    if (typeof id === "string" && id !== "") out[role] = id;
  }
  return out;
}

export function fromAgent(agent: Agent): AgentDraft {
  return {
    name: agent.name,
    description: agent.description,
    instructions: agent.instructions,
    kbId: agent.kbId,
    overrides: overridesOf(agent),
  };
}

/** The smallest PATCH that turns the stored Agent into the draft; `null` clears a role override. */
export function diffDraft(agent: Agent, draft: AgentDraft): AgentPatch {
  const base = fromAgent(agent);
  const patch: AgentPatch = {};
  if (draft.name.trim() !== base.name) patch.name = draft.name.trim();
  // A description of only spaces means "no description"; send it as empty, not as spaces.
  const description = draft.description.trim() === "" ? "" : draft.description;
  if (description !== base.description) patch.description = description;
  if (draft.instructions !== base.instructions) patch.instructions = draft.instructions;
  if (draft.kbId !== base.kbId) patch.kbId = draft.kbId;
  const overrides: Record<string, string | null> = {};
  for (const role of agentRoles) {
    if (base.overrides[role] !== draft.overrides[role]) overrides[role] = draft.overrides[role] ?? null;
  }
  if (Object.keys(overrides).length > 0) patch.modelOverrides = overrides as AgentPatch["modelOverrides"];
  return patch;
}

export function isDirty(agent: Agent, draft: AgentDraft): boolean {
  return Object.keys(diffDraft(agent, draft)).length > 0;
}

export type DraftErrors = {
  name?: "required" | "tooLong";
  description?: "tooLong";
  instructions?: "tooLong";
  kbId?: "required";
};

const runes = (value: string) => [...value].length;

/** Client-side checks that mirror core's validation, so an invalid draft never reaches the network. */
export function draftErrors(draft: AgentDraft): DraftErrors {
  const errors: DraftErrors = {};
  const name = draft.name.trim();
  if (name === "") errors.name = "required";
  else if (runes(name) > AGENT_NAME_MAX) errors.name = "tooLong";
  if (runes(draft.description) > AGENT_DESCRIPTION_MAX) errors.description = "tooLong";
  if (runes(draft.instructions) > AGENT_INSTRUCTIONS_MAX) errors.instructions = "tooLong";
  if (draft.kbId === "") errors.kbId = "required";
  return errors;
}

export function canSave(agent: Agent, draft: AgentDraft): boolean {
  return isDirty(agent, draft) && Object.keys(draftErrors(draft)).length === 0;
}
