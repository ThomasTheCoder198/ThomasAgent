"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/AlertDialog";
import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";

/** Asked when leaving a scope tab that holds unsaved role edits. */
export function DiscardDialog({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }) {
  const t = useTranslations("settings");
  return (
    <AlertDialog open onOpenChange={(open) => !open && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("discardTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("discardBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost">{t("keepEditing")}</AlertDialogCancel>
          <Button variant="destructive" onClick={onConfirm}>
            {t("discardConfirm")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/** The org tab has nothing to show without an org (or when the org list failed): say why, in place. */
export function NoOrg({ failed }: { failed: boolean }) {
  const t = useTranslations("settings");
  if (failed) return <InlineAlert title={t("orgRolesFailed")} />;
  return (
    <div className="flex flex-col items-start gap-3">
      <p className="text-muted-foreground text-sm">{t("noOrg")}</p>
      <Button asChild variant="outline" size="sm">
        <Link href="/orgs">{t("noOrgAction")}</Link>
      </Button>
    </div>
  );
}
