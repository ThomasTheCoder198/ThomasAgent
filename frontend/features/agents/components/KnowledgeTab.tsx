"use client";

import { useTranslations } from "next-intl";

import { Field } from "@/components/ui/Field";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import { SettingsSection } from "@/components/ui/SettingsSection";
import type { Loaded } from "@/lib/api/loaded";
import type { KnowledgeBase } from "@/lib/ragx/types";

import type { TabProps } from "./tabs";

type Props = TabProps & { knowledgeBases: Loaded<KnowledgeBase[]> };

/**
 * Exactly one KB per Agent. A load failure is shown as an error, never as an empty list; a binding to a KB the
 * list does not contain (deleted, or another tenant's) stays selectable as its own "unknown" item so it cannot hide.
 */
export function KnowledgeTab({ agent, draft, update, knowledgeBases }: Props) {
  const t = useTranslations("agents");
  if (!knowledgeBases.ok) {
    return (
      <SettingsSection title={t("tabKnowledge")} hint={t("kbHint")}>
        <InlineAlert title={t("kbLoadFailed")} detail={knowledgeBases.message} />
      </SettingsSection>
    );
  }
  const list = knowledgeBases.data;
  const storedIsKnown = list.some((kb) => kb.id === agent.kbId);
  const unknownSelected = !storedIsKnown && draft.kbId === agent.kbId;
  return (
    <SettingsSection title={t("tabKnowledge")} hint={t("kbHint")}>
      <div className="flex flex-col gap-3">
        <Field label={t("tabKnowledge")} hideLabel>
          {(control) => (
            <Select value={draft.kbId} onValueChange={(kbId) => update({ kbId })}>
              <SelectTrigger {...control} className="w-full max-w-md">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {!storedIsKnown && <SelectItem value={agent.kbId}>{t("kbUnknown")}</SelectItem>}
                {list.map((kb) => (
                  <SelectItem key={kb.id} value={kb.id}>
                    {kb.name} · {t("documents", { count: kb.fileCount })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
        {unknownSelected && (
          <InlineAlert
            title={agent.kbMissing ? t("kbDeleted") : t("kbUnknown")}
            detail={t("kbUnknownHint", { id: agent.kbId })}
          />
        )}
      </div>
    </SettingsSection>
  );
}
