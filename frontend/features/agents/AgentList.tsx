import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";

import { LineDots } from "@/components/metro/LineDots";
import { formatDateTime } from "@/lib/format";

import type { AgentSummary } from "./contract";

const COLUMNS = "md:grid-cols-[minmax(0,2.2fr)_minmax(0,1.2fr)_minmax(0,0.9fr)_minmax(0,1fr)_9.5rem_1rem]";

/** A departure board of Agents: one ruled row each, every column a fact the operator scans for. */
export async function AgentList({ agents }: { agents: AgentSummary[] }) {
  const t = await getTranslations("agents");
  const locale = await getLocale();
  if (agents.length === 0) return <p className="text-ink-2 py-10">{t("empty")}</p>;
  return (
    <div role="table" aria-label={t("title")} className="text-sm">
      <div
        role="row"
        className={`text-ink-3 border-rule hidden gap-5 border-b px-3 pb-2.5 text-xs font-medium md:grid ${COLUMNS}`}
      >
        <span role="columnheader">{t("colAgent")}</span>
        <span role="columnheader">{t("colKb")}</span>
        <span role="columnheader">{t("colLines")}</span>
        <span role="columnheader">{t("colModel")}</span>
        <span role="columnheader">{t("colUpdated")}</span>
        <span aria-hidden />
      </div>
      {agents.map((agent, index) => (
        <Link
          key={agent.id}
          href={`/agents/${encodeURIComponent(agent.id)}`}
          role="row"
          style={{ animationDelay: `calc(${index} * var(--stagger))` }}
          className={`animate-rise group border-rule-2 hover:bg-paper grid grid-cols-[1fr_auto] items-center gap-x-5 gap-y-1.5 border-b px-3 py-4 transition-colors ${COLUMNS}`}
        >
          <span role="cell" className="min-w-0">
            <span className="block truncate text-[15px] font-semibold">{agent.name}</span>
            <span className="text-ink-2 block truncate">{agent.description}</span>
          </span>
          <span
            role="cell"
            className="text-ink-2 col-start-1 flex min-w-0 items-center gap-2 md:col-start-auto"
          >
            <span aria-hidden className="bg-kb size-2 shrink-0 rounded-full" />
            <span className="truncate">{agent.kb.name}</span>
          </span>
          <span role="cell" className="text-ink-2 col-start-1 flex items-center gap-2 md:col-start-auto">
            <LineDots lines={agent.lines} size="md" />
            {t("toolCount", { count: agent.toolCount })}
          </span>
          <span role="cell" className="tabular text-ink-2 col-start-1 truncate text-xs md:col-start-auto">
            {agent.chatModel}
          </span>
          <span role="cell" className="tabular text-ink-3 col-start-1 text-xs md:col-start-auto">
            {formatDateTime(agent.updatedAt, locale)}
          </span>
          <ChevronRight
            aria-hidden
            className="text-ink-3 group-hover:text-ink col-start-2 row-span-5 row-start-1 size-4 transition-transform group-hover:translate-x-0.5 md:col-start-auto md:row-auto"
          />
        </Link>
      ))}
    </div>
  );
}
