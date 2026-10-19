import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";
import type { Agent, AgentCreate } from "@/lib/core/types";

import { AGENT_PATHS } from "./contract";

export function createAgent(orgId: string, input: AgentCreate, locale: Locale): Promise<Agent> {
  return apiFetch<Agent>(AGENT_PATHS.agents(orgId), { method: "POST", body: JSON.stringify(input), locale });
}

export async function deleteAgent(orgId: string, id: string, locale: Locale): Promise<void> {
  await apiFetch<undefined>(AGENT_PATHS.agent(orgId, id), { method: "DELETE", locale });
}
