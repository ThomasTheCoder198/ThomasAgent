/**
 * Core's Agents API (contracts/openapi/core.v1.yaml, tag Agents), org-scoped (R1). An Agent has exactly one
 * Knowledge Base. Tool policies arrive with M3a (R3); until then every KB tool runs automatically.
 */
import type { Role } from "@/features/registry/types";
import type { Agent } from "@/lib/core/types";

const enc = encodeURIComponent;

export const AGENT_PATHS = {
  agents: (orgId: string) => `/api/v1/orgs/${enc(orgId)}/agents`,
  agent: (orgId: string, id: string) => `/api/v1/orgs/${enc(orgId)}/agents/${enc(id)}`,
} as const;

/** Mirror the DB CHECKs (index migration) so the form refuses early; core stays the authority. */
export const AGENT_NAME_MAX = 80;
export const AGENT_DESCRIPTION_MAX = 280;
export const AGENT_INSTRUCTIONS_MAX = 8000;

/** Roles an Agent may override; embedding and rerank belong to ragx, decision to the org (O6). */
export const agentRoles = ["chat.default", "chat.fast", "vision"] as const satisfies readonly Role[];
export type AgentRole = (typeof agentRoles)[number];

/** A list row: the Agent plus the display name of the model its `chat.default` resolves to. */
export type AgentRow = { agent: Agent; chatModel: string };

export const agentPage = (id: string) => `/agents/${enc(id)}`;
