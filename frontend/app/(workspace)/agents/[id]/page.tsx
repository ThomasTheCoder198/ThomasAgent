import { notFound } from "next/navigation";

import { AgentSettings } from "@/features/agents/AgentSettings";
import { getAgentPage } from "@/features/agents/server";
import { isAgentTab } from "@/features/agents/tab-ids";
import { ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { NoOrgState } from "@/features/orgs/NoOrgState";
import { getCurrentOrg } from "@/features/orgs/server";

export default async function AgentPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ tab?: string }>;
}) {
  const { org, failed, requestId } = await getCurrentOrg();
  if (failed) return <ServiceUnavailable requestId={requestId} />; // a 5xx is not "no organization" (frontend rule 9)
  if (!org) return <NoOrgState />;
  const [{ id }, { tab }] = await Promise.all([params, searchParams]);
  const data = await getAgentPage(org.id, decodeURIComponent(id));
  if (!data) notFound();
  return (
    <div className="h-full overflow-y-auto">
      <AgentSettings key={data.agent.id} {...data} initialTab={isAgentTab(tab) ? tab : "overview"} />
    </div>
  );
}
