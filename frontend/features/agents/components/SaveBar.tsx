"use client";

import { Check } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import type { DraftErrors } from "../draft";
import type { SaveState } from "../useAgentEditor";

type Props = {
  dirty: boolean;
  canSave: boolean;
  errors: DraftErrors;
  save: SaveState;
  onSave: () => void;
  onDiscard: () => void;
};

function StatusText({ save, errors }: Pick<Props, "save" | "errors">) {
  const t = useTranslations("agents");
  if (save.status === "saved") {
    return (
      <span className="flex items-center gap-2 text-sm font-medium">
        <Check aria-hidden className="text-kb animate-pop size-4" />
        {t("saved")}
      </span>
    );
  }
  if (save.status === "error") return <span className="text-app text-sm font-medium">{save.message}</span>;
  if (errors.name) return <span className="text-app text-sm font-medium">{t("nameRequired")}</span>;
  return <span className="text-sign-ink-2 text-sm">{t("unsaved")}</span>;
}

/**
 * Rises from the bottom only while something is unsaved (or just saved). When hidden it leaves the
 * accessibility tree entirely (`inert` + `aria-hidden`), and its live region only speaks real changes.
 */
export function SaveBar({ dirty, canSave, errors, save, onSave, onDiscard }: Props) {
  const t = useTranslations("agents");
  const visible = dirty || save.status === "saved" || save.status === "error";
  return (
    <div
      inert={!visible}
      aria-hidden={!visible || undefined}
      className={cn(
        "ease-out-expo pointer-events-none sticky bottom-0 z-10 px-4 pb-4 transition-[transform,opacity] duration-300 md:px-8",
        visible ? "translate-y-0 opacity-100" : "translate-y-full opacity-0",
      )}
    >
      <div className="bg-sign text-sign-ink shadow-lift pointer-events-auto mx-auto flex max-w-272 flex-wrap items-center gap-3 rounded-xl px-4 py-3">
        <div aria-live="polite" className="min-w-0">
          {visible && <StatusText save={save} errors={errors} />}
        </div>
        {dirty && (
          <div className="ml-auto flex gap-2">
            <Button
              variant="ghost"
              onClick={onDiscard}
              disabled={save.status === "saving"}
              className="text-sign-ink-2 hover:bg-sign-active hover:text-sign-ink"
            >
              {t("discard")}
            </Button>
            <Button
              onClick={onSave}
              disabled={!canSave || save.status === "saving"}
              pending={save.status === "saving"}
              className="bg-sign-ink text-sign hover:bg-sign-ink/90"
            >
              {save.status === "saving" ? t("saving") : t("save")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
