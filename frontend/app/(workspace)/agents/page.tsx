import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";

import { InlineAlert } from "@/components/ui/InlineAlert";
import { AgentList } from "@/features/agents/AgentList";
import type { AgentRow } from "@/features/agents/contract";
import { listAgents } from "@/features/agents/server";
import { getModelChoice } from "@/features/chat/server";
import { NewAgentButton } from "@/features/agents/components/NewAgentDialog";
import { listKnowledgeBases, type KbLoad } from "@/features/knowledge-bases/server";
import { LoadFailure, ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { NoOrgState } from "@/features/orgs/NoOrgState";
import { getCurrentOrg } from "@/features/orgs/server";
import { PageHeader } from "@/features/shell/PageHeader";

function kbRouteError(kbs: KbLoad<unknown>) {
  if (kbs.ok) return null;
  if (kbs.notFound) notFound();
  return kbs.serviceDown ? <ServiceUnavailable requestId={kbs.requestId} /> : null;
}

export default async function AgentsPage() {
  const { org, failed, requestId } = await getCurrentOrg();
  if (failed) return <ServiceUnavailable requestId={requestId} />; // a 5xx is not "no organization" (frontend rule 9)
  if (!org) return <NoOrgState />;
  const [t, agents, choice, kbs] = await Promise.all([
    getTranslations("agents"),
    listAgents(org.id),
    getModelChoice(org.id),
    listKnowledgeBases(org.id),
  ]);
  const kbError = kbRouteError(kbs);
  if (kbError) return kbError;
  const name = (id: string | undefined) => choice.models.find((m) => m.id === id)?.displayName ?? "";
  const rows: AgentRow[] = agents.ok
    ? agents.data.map((agent) => ({
        agent,
        chatModel: name(
          (agent.modelOverrides as Record<string, string> | undefined)?.["chat.default"] ??
            choice.defaultModelId,
        ),
      }))
    : [];
  return (
    <div className="h-full overflow-y-auto">
      <PageHeader
        title={t("title")}
        body={t("subtitle")}
        actions={kbs.ok ? <NewAgentButton orgId={org.id} knowledgeBases={kbs.data} /> : undefined}
      />
      <div className="mx-auto w-full max-w-5xl px-4 pb-16 md:px-6 lg:px-8">
        {!kbs.ok ? (
          <LoadFailure message={kbs.message} requestId={kbs.requestId} />
        ) : agents.ok ? (
          <AgentList rows={rows} />
        ) : (
          <InlineAlert title={t("loadFailed")} detail={agents.message} />
        )}
      </div>
    </div>
  );
}
