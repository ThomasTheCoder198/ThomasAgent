"use client";

import { FileText } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { SettingsSection } from "@/components/ui/SettingsSection";
import { TextArea } from "@/components/ui/TextArea";
import { TextField } from "@/components/ui/TextField";
import { formatBytes, formatTokens } from "@/lib/format";

import type { AgentDetail } from "../contract";
import type { AgentDraft, DraftErrors } from "../draft";

export type TabProps = {
  agent: AgentDetail;
  draft: AgentDraft;
  update: (patch: Partial<AgentDraft>) => void;
  errors: DraftErrors;
};

export function OverviewTab({ agent, draft, update, errors }: TabProps) {
  const t = useTranslations("agents");
  const locale = useLocale();
  return (
    <>
      <SettingsSection title={t("name")}>
        <TextField
          label={t("name")}
          value={draft.name}
          onChange={(e) => update({ name: e.target.value })}
          error={errors.name ? t("nameRequired") : undefined}
          required
        />
      </SettingsSection>
      <SettingsSection title={t("description")}>
        <TextArea
          label={t("description")}
          hideLabel
          rows={2}
          value={draft.description}
          onChange={(e) => update({ description: e.target.value })}
        />
      </SettingsSection>
      <SettingsSection title={t("agentId")}>
        <code className="tabular text-ink-2 bg-paper border-rule rounded-md border px-2.5 py-1.5 text-[13px]">
          {agent.id}
        </code>
      </SettingsSection>
      <SettingsSection title={t("contextFiles")} hint={t("contextFilesHint")}>
        {agent.contextFiles.length === 0 ? (
          <p className="text-ink-3 text-sm">{t("noContextFiles")}</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {agent.contextFiles.map((file) => (
              <li key={file.id} className="flex items-center gap-2.5 text-sm">
                <FileText aria-hidden className="text-ink-3 size-4" />
                <span className="min-w-0 flex-1 truncate">{file.name}</span>
                <span className="tabular text-ink-3 text-xs">{formatBytes(file.sizeBytes, locale)}</span>
              </li>
            ))}
          </ul>
        )}
      </SettingsSection>
    </>
  );
}

export function InstructionsTab({ draft, update }: TabProps) {
  const t = useTranslations("agents");
  const locale = useLocale();
  return (
    <SettingsSection title={t("tabInstructions")} hint={t("instructionsHint")}>
      <TextArea
        label={t("tabInstructions")}
        hideLabel
        value={draft.instructions}
        onChange={(e) => update({ instructions: e.target.value })}
        className="min-h-72"
        footer={t("chars", { count: formatTokens(draft.instructions.length, locale) })}
      />
    </SettingsSection>
  );
}
