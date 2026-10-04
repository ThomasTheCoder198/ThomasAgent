import "server-only";

import { REGISTRY_PATHS, type Model, type RoleAssignment } from "@/features/registry/types";
import { serverFetchOptional, serverFetchResult, type Loaded } from "@/lib/api/server";

import { AGENT_PATHS, type AgentDetail, type AgentSummary, type KnowledgeBaseSummary } from "./contract";

/** The list may be empty before core serves Agents (M2): a 404 there reads as "no agents yet". */
export function listAgents(): Promise<AgentSummary[]> {
  return serverFetchOptional(AGENT_PATHS.agents, []);
}

export type AgentPageData = {
  agent: AgentDetail;
  knowledgeBases: Loaded<KnowledgeBaseSummary[]>;
  models: Loaded<Model[]>;
  roles: Loaded<RoleAssignment[]>;
};

/** Supporting lists never silently become empty: a failure is carried to the tab that needs it. */
export async function getAgentPage(id: string): Promise<AgentPageData | null> {
  const [agent, knowledgeBases, models, roles] = await Promise.all([
    serverFetchOptional<AgentDetail | null>(AGENT_PATHS.agent(id), null),
    serverFetchResult<KnowledgeBaseSummary[]>(AGENT_PATHS.knowledgeBases),
    serverFetchResult<Model[]>(REGISTRY_PATHS.models),
    serverFetchResult<RoleAssignment[]>(REGISTRY_PATHS.roles),
  ]);
  return agent ? { agent, knowledgeBases, models, roles } : null;
}
