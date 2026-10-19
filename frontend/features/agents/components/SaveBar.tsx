"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import type { DraftErrors } from "../draft";

type Props = {
  dirty: boolean;
  canSave: boolean;
  errors: DraftErrors;
  saving: boolean;
  onSave: () => void;
  onDiscard: () => void;
};

/**
 * Rises from the bottom only while something is unsaved. When hidden it leaves the accessibility tree
 * entirely (`inert` + `aria-hidden`), and its live region only speaks real changes. The save outcome is a
 * toast (success or the catalog error), so the bar itself only ever says "unsaved" or why Save is blocked.
 */
export function SaveBar({ dirty, canSave, errors, saving, onSave, onDiscard }: Props) {
  const t = useTranslations("agents");
  return (
    <div
      inert={!dirty}
      aria-hidden={!dirty || undefined}
      data-save-bar-visible={dirty || undefined}
      className={cn(
        "ease-out-expo pointer-events-none sticky bottom-0 z-10 px-4 pb-4 transition-[transform,opacity] duration-300 md:px-6 lg:px-8",
        dirty ? "translate-y-0 opacity-100" : "translate-y-full opacity-0",
      )}
    >
      <div className="bg-card border-outline shadow-lift pointer-events-auto mx-auto flex max-w-5xl flex-wrap items-center gap-3 rounded-lg border px-4 py-3">
        <div aria-live="polite" className="min-w-0">
          {dirty &&
            (errors.name ? (
              <span className="text-destructive text-sm font-medium">{t("nameRequired")}</span>
            ) : (
              <span className="text-muted-foreground text-sm">{t("unsaved")}</span>
            ))}
        </div>
        <div className="ml-auto flex gap-2">
          <Button variant="ghost" onClick={onDiscard} disabled={saving}>
            {t("discard")}
          </Button>
          <Button onClick={onSave} disabled={!canSave || saving} pending={saving}>
            {saving ? t("saving") : t("save")}
          </Button>
        </div>
      </div>
    </div>
  );
}
