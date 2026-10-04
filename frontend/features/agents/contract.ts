/**
 * PROVISIONAL M2 Agent contract, shared with `mock-core/` until core serves Agents and the shapes land in
 * contracts/openapi/core.v1.yaml. An Agent has exactly one Knowledge Base (decision 2026-10-04); tool
 * policies follow spec §8 (auto / ask / off, risky tools default to ask).
 */
import type { ToolLine } from "@/components/metro/lines";
import type { Role } from "@/features/registry/types";

export const AGENT_PATHS = {
  agents: "/api/v1/agents",
  agent: (id: string) => `/api/v1/agents/${encodeURIComponent(id)}`,
  knowledgeBases: "/api/v1/knowledge-bases",
} as const;

export const toolPolicies = ["auto", "ask", "off"] as const;
export type ToolPolicy = (typeof toolPolicies)[number];

/** Roles an Agent may override; embedding and rerank belong to the Knowledge Base, not the Agent. */
export const agentRoles = ["chat.default", "chat.fast", "vision"] as const satisfies readonly Role[];
export type AgentRole = (typeof agentRoles)[number];

export type AgentTool = {
  id: string;
  name: string;
  title: string;
  description: string;
  risky: boolean;
  policy: ToolPolicy;
};

export type AgentToolSource = {
  id: string;
  line: ToolLine;
  kind: "app" | "mcp" | "composio";
  name: string;
  tools: AgentTool[];
};

export type ContextFile = { id: string; name: string; sizeBytes: number };

export type AgentSummary = {
  id: string;
  name: string;
  description: string;
  kb: { id: string; name: string };
  lines: ToolLine[];
  toolCount: number;
  /** Display name of the model the `chat.default` role resolves to for this Agent. */
  chatModel: string;
  updatedAt: string;
};

export type AgentDetail = {
  id: string;
  name: string;
  description: string;
  instructions: string;
  kbId: string;
  toolSources: AgentToolSource[];
  /** Missing role = inherit the platform assignment. */
  modelOverrides: Partial<Record<AgentRole, string>>;
  contextFiles: ContextFile[];
  updatedAt: string;
};

export type KnowledgeBaseSummary = { id: string; name: string; documents: number; updatedAt: string };

export type AgentPatch = {
  name?: string;
  description?: string;
  instructions?: string;
  kbId?: string;
  toolPolicies?: Record<string, ToolPolicy>;
  /** `null` clears an override. */
  modelOverrides?: Partial<Record<AgentRole, string | null>>;
};
