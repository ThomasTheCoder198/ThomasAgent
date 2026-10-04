"use client";

import { ShieldAlert } from "lucide-react";
import { useTranslations } from "next-intl";

import { LineBadge } from "@/components/metro/LineBadge";
import { lineIcons } from "@/components/metro/lineIcons";
import { Segmented } from "@/components/ui/Segmented";

import { toolPolicies, type AgentTool, type AgentToolSource, type ToolPolicy } from "../contract";
import type { TabProps } from "./tabs";

const policyKey = { auto: "policyAuto", ask: "policyAsk", off: "policyOff" } as const;
const kindKey = { mcp: "kindMcp", composio: "kindComposio", app: "kindApp" } as const;

function ToolRow({
  tool,
  policy,
  onChange,
}: {
  tool: AgentTool;
  policy: ToolPolicy;
  onChange: (p: ToolPolicy) => void;
}) {
  const t = useTranslations("agents");
  return (
    <li className="border-rule-2 flex flex-col gap-3 border-b py-3.5 last:border-b-0 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
          <span className="text-sm font-semibold">{tool.title}</span>
          <code className="tabular text-ink-3 text-xs">{tool.name}</code>
          {tool.risky && (
            <span className="text-app inline-flex items-center gap-1 text-xs font-medium">
              <ShieldAlert aria-hidden className="size-3.5" />
              {t("risky")}
            </span>
          )}
        </div>
        <p className="text-ink-2 mt-0.5 text-[13.5px]">{tool.description}</p>
      </div>
      <div className="w-full sm:w-72">
        <Segmented<ToolPolicy>
          name={`policy-${tool.id}`}
          legend={t("policyLegend", { tool: tool.title })}
          hideLegend
          tone="ground"
          value={policy}
          onChange={onChange}
          options={toolPolicies.map((value) => ({ value, label: t(policyKey[value]) }))}
        />
      </div>
    </li>
  );
}

function SourceGroup({
  source,
  draft,
  update,
}: { source: AgentToolSource } & Pick<TabProps, "draft" | "update">) {
  const t = useTranslations("agents");
  return (
    <section className="animate-rise">
      <header className="flex items-center gap-2.5">
        <LineBadge line={source.line} icon={lineIcons[source.line]}>
          {source.name}
        </LineBadge>
        <span className="text-ink-3 text-xs">{t(kindKey[source.kind])}</span>
      </header>
      <ul className="mt-1">
        {source.tools.map((tool) => (
          <ToolRow
            key={tool.id}
            tool={tool}
            policy={draft.policies[tool.id] ?? tool.policy}
            onChange={(policy) => update({ policies: { ...draft.policies, [tool.id]: policy } })}
          />
        ))}
      </ul>
    </section>
  );
}

export function ToolsTab({ agent, draft, update }: TabProps) {
  const t = useTranslations("agents");
  return (
    <div className="flex flex-col gap-8 pt-2">
      <p className="text-ink-2 max-w-[70ch] text-[13.5px]">{t("toolsHint")}</p>
      {agent.toolSources.map((source) => (
        <SourceGroup key={source.id} source={source} draft={draft} update={update} />
      ))}
    </div>
  );
}
