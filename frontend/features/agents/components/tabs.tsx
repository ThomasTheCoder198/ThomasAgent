"use client";

import { FileText } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { SettingsSection } from "@/components/ui/SettingsSection";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
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
        <Field label={t("name")} error={errors.name ? t("nameRequired") : undefined}>
          {(control) => (
            <Input
              {...control}
              value={draft.name}
              onChange={(e) => update({ name: e.target.value })}
              required
            />
          )}
        </Field>
      </SettingsSection>
      <SettingsSection title={t("description")}>
        <Field label={t("description")} hideLabel>
          {(control) => (
            <Textarea
              {...control}
              rows={2}
              value={draft.description}
              onChange={(e) => update({ description: e.target.value })}
            />
          )}
        </Field>
      </SettingsSection>
      <SettingsSection title={t("agentId")}>
        <code className="tabular text-muted-foreground bg-card border-border rounded-md border px-2.5 py-1.5 text-[13px]">
          {agent.id}
        </code>
      </SettingsSection>
      <SettingsSection title={t("contextFiles")} hint={t("contextFilesHint")}>
        {agent.contextFiles.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t("noContextFiles")}</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {agent.contextFiles.map((file) => (
              <li key={file.id} className="flex items-center gap-2.5 text-sm">
                <FileText aria-hidden className="text-muted-foreground size-4" />
                <span className="min-w-0 flex-1 truncate">{file.name}</span>
                <span className="tabular text-muted-foreground text-xs">
                  {formatBytes(file.sizeBytes, locale)}
                </span>
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
      <Field
        label={t("tabInstructions")}
        hideLabel
        footer={t("chars", { count: formatTokens(draft.instructions.length, locale) })}
      >
        {(control) => (
          <Textarea
            {...control}
            value={draft.instructions}
            onChange={(e) => update({ instructions: e.target.value })}
            className="min-h-72"
          />
        )}
      </Field>
    </SettingsSection>
  );
}
