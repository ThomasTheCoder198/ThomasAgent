"use client";

import { LibraryBig } from "lucide-react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { lineText, lineTint } from "@/components/lines";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { orgHome } from "@/features/orgs/contract";
import type { Locale } from "@/i18n/config";
import { cn } from "@/lib/cn";
import type { KnowledgeBase } from "@/lib/ragx/types";

import { deleteKnowledgeBase } from "../api";
import { ConfirmDelete } from "./ConfirmDelete";
import { KbPageHeader } from "./KbPageHeader";
import { RenameKnowledgeBaseDialog } from "./KnowledgeBaseDialogs";

/** The KB's page header: name, its badge, count and model in mono, then Rename and (not for the default) Delete. */
export function KnowledgeBaseHeader({
  orgId,
  knowledgeBase,
}: {
  orgId: string;
  knowledgeBase: KnowledgeBase;
}) {
  const t = useTranslations("kb");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const close = (open: boolean) => !open && setDialog(null);
  return (
    <>
      <KbPageHeader
        title={knowledgeBase.name}
        back={{ href: orgHome(orgId), label: t("back") }}
        meta={
          <>
            <Badge className={cn(lineTint.kb, lineText.kb)}>
              <LibraryBig aria-hidden />
              {t("plate")}
            </Badge>
            {knowledgeBase.isDefault && <Badge variant="outline">{t("default")}</Badge>}
            <span className="tabular text-muted-foreground text-xs">
              {t("files", { count: knowledgeBase.fileCount })}
            </span>
            <span className="tabular text-muted-foreground text-xs">
              {knowledgeBase.embeddingModel} · {knowledgeBase.embeddingDims}
            </span>
          </>
        }
        actions={
          <>
            <Button variant="outline" onClick={() => setDialog("rename")}>
              {t("rename")}
            </Button>
            {!knowledgeBase.isDefault && (
              <Button
                variant="outline"
                className="border-destructive text-destructive hover:bg-destructive/10"
                onClick={() => setDialog("delete")}
              >
                {t("delete")}
              </Button>
            )}
          </>
        }
      />
      {dialog === "rename" && (
        <RenameKnowledgeBaseDialog orgId={orgId} knowledgeBase={knowledgeBase} onOpenChange={close} />
      )}
      {dialog === "delete" && (
        <ConfirmDelete
          onOpenChange={close}
          title={t("deleteTitle", { name: knowledgeBase.name })}
          body={t("deleteBody")}
          successMessage={t("toast.deleted", { name: knowledgeBase.name })}
          onConfirm={async () => {
            await deleteKnowledgeBase(orgId, knowledgeBase.id, locale);
            router.push(orgHome(orgId));
          }}
        />
      )}
    </>
  );
}
