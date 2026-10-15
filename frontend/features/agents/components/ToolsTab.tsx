"use client";

import { ShieldAlert } from "lucide-react";
import { useTranslations } from "next-intl";

import { LineBadge } from "@/components/LineBadge";
import { lineIcons } from "@/components/lineIcons";
import { SegmentedChoice } from "@/components/ui/SegmentedChoice";

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
    <li className="border-border grid gap-4 border-b py-6 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-8">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h3 className="text-base font-semibold">{tool.title}</h3>
          {tool.risky && (
            <span className="text-destructive inline-flex items-center gap-1 text-xs font-medium">
              <ShieldAlert aria-hidden className="size-3.5" />
              {t("risky")}
            </span>
          )}
        </div>
        <code className="tabular text-muted-foreground mt-1 block text-xs break-all">{tool.name}</code>
        <p className="text-muted-foreground mt-1 text-sm leading-relaxed">{tool.description}</p>
      </div>
      <div className="min-w-0 sm:max-w-sm">
        <SegmentedChoice<ToolPolicy>
          legend={t("policyLegend", { tool: tool.title })}
          hideLegend
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
      <header className="border-border flex items-center gap-3 border-b pb-3">
        <h2 className="contents">
          <LineBadge line={source.line} icon={lineIcons[source.line]}>
            {source.name}
          </LineBadge>
        </h2>
        <span className="text-muted-foreground text-sm">{t(kindKey[source.kind])}</span>
      </header>
      <ul>
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
    <div className="flex flex-col gap-12">
      <p className="text-muted-foreground max-w-[70ch] text-sm">{t("toolsHint")}</p>
      {agent.toolSources.map((source) => (
        <SourceGroup key={source.id} source={source} draft={draft} update={update} />
      ))}
    </div>
  );
}
