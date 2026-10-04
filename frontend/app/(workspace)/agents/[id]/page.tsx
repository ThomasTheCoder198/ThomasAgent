import { notFound } from "next/navigation";

import { AgentSettings } from "@/features/agents/AgentSettings";
import { getAgentPage } from "@/features/agents/server";
import { isAgentTab } from "@/features/agents/tab-ids";

export default async function AgentPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ tab?: string }>;
}) {
  const [{ id }, { tab }] = await Promise.all([params, searchParams]);
  const data = await getAgentPage(decodeURIComponent(id));
  if (!data) notFound();
  return (
    <div className="h-full overflow-y-auto">
      <AgentSettings key={data.agent.id} {...data} initialTab={isAgentTab(tab) ? tab : "overview"} />
    </div>
  );
}
