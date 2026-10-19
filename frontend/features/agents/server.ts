import "server-only";

import { RAGX_PATHS } from "@/features/knowledge-bases/contract";
import { REGISTRY_PATHS, type Model } from "@/features/registry/types";
import { ORG_ROLE_PATHS } from "@/features/settings/contract";
import { serverFetchOptional, serverFetchResult, type Loaded } from "@/lib/api/server";
import type { Agent, OrgModelRole } from "@/lib/core/types";
import type { KnowledgeBase } from "@/lib/ragx/types";

import { AGENT_PATHS } from "./contract";

export function listAgents(orgId: string): Promise<Loaded<Agent[]>> {
  return serverFetchResult<Agent[]>(AGENT_PATHS.agents(orgId));
}

export type AgentPageData = {
  orgId: string;
  agent: Agent;
  knowledgeBases: Loaded<KnowledgeBase[]>;
  models: Loaded<Model[]>;
  roles: Loaded<OrgModelRole[]>;
};

/** Supporting lists never silently become empty: a failure is carried to the tab that needs it. */
export async function getAgentPage(orgId: string, id: string): Promise<AgentPageData | null> {
  const [agent, knowledgeBases, models, roles] = await Promise.all([
    serverFetchOptional<Agent | null>(AGENT_PATHS.agent(orgId, id), null),
    serverFetchResult<KnowledgeBase[]>(RAGX_PATHS.knowledgeBases(orgId)),
    serverFetchResult<Model[]>(REGISTRY_PATHS.models),
    serverFetchResult<OrgModelRole[]>(ORG_ROLE_PATHS.roles(orgId)),
  ]);
  return agent ? { orgId, agent, knowledgeBases, models, roles } : null;
}
