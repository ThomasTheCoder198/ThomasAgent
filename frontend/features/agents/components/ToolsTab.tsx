"use client";

import { useTranslations } from "next-intl";

import { LineBadge } from "@/components/LineBadge";
import { SettingsSection } from "@/components/ui/SettingsSection";
import { KB_TOOL_NAMES } from "@/features/chat/contract";

import type { TabProps } from "./tabs";

const toolKey = {
  kb_search: "toolKbSearch",
  kb_outline: "toolKbOutline",
  kb_read_section: "toolKbReadSection",
  kb_list_documents: "toolKbListDocuments",
} as const;

/** M2 has only the read-only KB tools, all automatic (R3); policies per tool arrive with M3a. */
export function ToolsTab({ agent }: TabProps) {
  const t = useTranslations("agents");
  return (
    <SettingsSection title={t("tabTools")} hint={t("toolsReadOnlyHint")}>
      <div className="flex flex-col gap-3">
        <div>
          <LineBadge line="kb">{agent.kbName || t("kbUnknown")}</LineBadge>
        </div>
        <ul className="border-border divide-border divide-y rounded-sm border">
          {KB_TOOL_NAMES.map((name) => (
            <li key={name} className="flex min-h-11 items-center justify-between gap-4 px-3 py-3 text-sm">
              <span className="min-w-0">
                <span className="block font-medium">{t(toolKey[name])}</span>
                <code className="tabular text-muted-foreground text-xs">{name}</code>
              </span>
              <span className="text-muted-foreground shrink-0 text-xs">{t("policyAuto")}</span>
            </li>
          ))}
        </ul>
      </div>
    </SettingsSection>
  );
}
