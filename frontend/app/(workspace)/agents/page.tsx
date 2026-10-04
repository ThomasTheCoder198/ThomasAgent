import { getTranslations } from "next-intl/server";

import { AgentList } from "@/features/agents/AgentList";
import { listAgents } from "@/features/agents/server";
import { PageHeader } from "@/features/shell/PageHeader";

export default async function AgentsPage() {
  const [t, agents] = await Promise.all([getTranslations("agents"), listAgents()]);
  return (
    <div className="h-full overflow-y-auto">
      <PageHeader title={t("title")} body={t("subtitle")} />
      <div className="mx-auto w-full max-w-272 px-4 pb-16 md:px-8">
        <AgentList agents={agents} />
      </div>
    </div>
  );
}
