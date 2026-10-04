"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { InlineAlert } from "@/components/ui/InlineAlert";
import { SettingsSection } from "@/components/ui/SettingsSection";
import type { Loaded } from "@/lib/api/loaded";
import { cn } from "@/lib/cn";

import type { KnowledgeBaseSummary } from "../contract";
import type { TabProps } from "./tabs";

type OptionProps = {
  id: string;
  selected: boolean;
  onSelect: () => void;
  tone?: "kb" | "warn";
  children: ReactNode;
};

function KbOption({ id, selected, onSelect, tone = "kb", children }: OptionProps) {
  const warn = tone === "warn";
  return (
    <label
      className={cn(
        "has-focus-visible:outline-focus flex cursor-pointer items-center gap-3 rounded-lg border px-3.5 py-3 transition-[background-color,border-color] duration-200 has-focus-visible:outline-2",
        selected && !warn && "border-kb bg-kb-tint",
        selected && warn && "border-app bg-app-tint",
        !selected && "border-rule hover:border-rule-strong",
      )}
    >
      <input type="radio" name="kb" value={id} checked={selected} onChange={onSelect} className="sr-only" />
      <span
        aria-hidden
        className={cn(
          "bg-paper size-4.5 shrink-0 rounded-full transition-[border-width,border-color] duration-200",
          selected ? "border-[5px]" : "border-rule-strong border-2",
          selected && (warn ? "border-app" : "border-kb"),
        )}
      />
      {children}
    </label>
  );
}

type Props = TabProps & { knowledgeBases: Loaded<KnowledgeBaseSummary[]> };

/**
 * Exactly one KB per Agent. A load failure is shown as an error, never as an empty list; a binding to a KB the
 * list does not contain (deleted, or another tenant's) is shown as its own "unknown" row so it cannot hide.
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
  return (
    <SettingsSection title={t("tabKnowledge")} hint={t("kbHint")}>
      <fieldset>
        <legend className="sr-only">{t("tabKnowledge")}</legend>
        <div className="flex flex-col gap-1.5">
          {!storedIsKnown && (
            <KbOption
              id={agent.kbId}
              tone="warn"
              selected={draft.kbId === agent.kbId}
              onSelect={() => update({ kbId: agent.kbId })}
            >
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium">{t("kbUnknown")}</span>
                <span className="text-ink-2 block text-xs">{t("kbUnknownHint", { id: agent.kbId })}</span>
              </span>
            </KbOption>
          )}
          {list.map((kb) => (
            <KbOption
              key={kb.id}
              id={kb.id}
              selected={draft.kbId === kb.id}
              onSelect={() => update({ kbId: kb.id })}
            >
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{kb.name}</span>
              <span className="tabular text-ink-3 text-xs">{t("documents", { count: kb.documents })}</span>
            </KbOption>
          ))}
        </div>
      </fieldset>
    </SettingsSection>
  );
}
