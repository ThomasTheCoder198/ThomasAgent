"use client";

import { useLocale, useTranslations } from "next-intl";

import { SettingsSection } from "@/components/ui/SettingsSection";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import type { Agent } from "@/lib/core/types";
import { formatTokens } from "@/lib/format";

import type { AgentDraft, DraftErrors } from "../draft";
import { DeleteAgentSection } from "./DeleteAgentSection";

export type TabProps = {
  orgId: string;
  agent: Agent;
  draft: AgentDraft;
  update: (patch: Partial<AgentDraft>) => void;
  errors: DraftErrors;
};

export function OverviewTab({ orgId, agent, draft, update, errors }: TabProps) {
  const t = useTranslations("agents");
  return (
    <>
      <SettingsSection index={0} title={t("name")}>
        <Field
          label={t("name")}
          error={errors.name ? t(errors.name === "tooLong" ? "nameTooLong" : "nameRequired") : undefined}
        >
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
      <SettingsSection index={1} title={t("description")}>
        <Field
          label={t("description")}
          hideLabel
          error={errors.description ? t("descriptionTooLong") : undefined}
        >
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
      <SettingsSection index={2} title={t("agentId")}>
        <code className="tabular text-muted-foreground bg-card border-border rounded-md border px-2.5 py-1.5 text-[13px]">
          {agent.id}
        </code>
      </SettingsSection>
      <DeleteAgentSection orgId={orgId} id={agent.id} name={agent.name} index={3} />
    </>
  );
}

export function InstructionsTab({ draft, update, errors }: TabProps) {
  const t = useTranslations("agents");
  const locale = useLocale();
  return (
    <SettingsSection title={t("tabInstructions")} hint={t("instructionsHint")}>
      <Field
        label={t("tabInstructions")}
        hideLabel
        error={errors.instructions ? t("instructionsTooLong") : undefined}
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
