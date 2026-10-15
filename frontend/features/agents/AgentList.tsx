import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { getTranslations } from "next-intl/server";

import { EmptyState } from "@/features/shell/EmptyState";

import { AgentMeta } from "./components/AgentMeta";
import type { AgentSummary } from "./contract";

/**
 * Ruled rows, one per Agent, the whole row a link. Wide screens put the name beside what the Agent is built
 * from; on phones the facts stack under the name so nothing is squeezed.
 */
export async function AgentList({ agents }: { agents: AgentSummary[] }) {
  const t = await getTranslations("agents");
  if (agents.length === 0)
    return <EmptyState level="h2" line="model" title={t("empty")} body={t("subtitle")} />;
  return (
    <ul aria-label={t("title")} className="-mx-3">
      {agents.map((agent, index) => (
        <li
          key={agent.id}
          style={{ animationDelay: `calc(${index} * var(--stagger))` }}
          className="animate-rise"
        >
          <Link
            href={`/agents/${encodeURIComponent(agent.id)}`}
            className="group border-border hover:bg-accent/50 flex min-h-11 items-center gap-4 border-b px-3 py-3 transition-colors md:py-4"
          >
            <span className="flex min-w-0 flex-1 flex-col gap-2 md:flex-row md:items-center md:gap-6">
              <span className="min-w-0 md:flex-1">
                <span className="block truncate text-base font-semibold">{agent.name}</span>
                {agent.description && (
                  <span className="text-muted-foreground block truncate text-sm">{agent.description}</span>
                )}
              </span>
              <span className="min-w-0 md:max-w-[55%] md:shrink-0">
                <AgentMeta
                  kbName={agent.kb.name}
                  lines={agent.lines}
                  toolCount={agent.toolCount}
                  model={agent.chatModel}
                />
              </span>
            </span>
            <ChevronRight
              aria-hidden
              className="text-muted-foreground group-hover:text-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5"
            />
          </Link>
        </li>
      ))}
    </ul>
  );
}
