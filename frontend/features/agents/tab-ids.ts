/** Settings tabs of an Agent; shared by the server page (URL `?tab=`) and the client tabs. */
export const agentTabs = ["overview", "instructions", "knowledge", "tools", "models"] as const;
export type AgentTab = (typeof agentTabs)[number];

/** Query parameter that keeps the open tab in the URL. */
export const TAB_QUERY_KEY = "tab";

export function isAgentTab(value: string | undefined): value is AgentTab {
  return agentTabs.includes(value as AgentTab);
}
