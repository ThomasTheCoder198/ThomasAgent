import type { Agent } from "@/lib/core/types";

import type { ChatScope } from "./contract";

/** What a new chat with this Agent can reach (R12): its one KB on the KB line. M3a adds other sources. */
export function scopeOfAgent(agent: Agent): ChatScope {
  return {
    agent: { id: agent.id, name: agent.name },
    kb: { id: agent.kbId, name: agent.kbName, missing: agent.kbMissing === true },
    toolSources: [{ line: "kb", name: agent.kbName }],
  };
}
