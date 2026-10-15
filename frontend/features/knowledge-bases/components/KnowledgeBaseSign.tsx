"use client";

import { ArrowLeft, LibraryBig } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { LineBadge } from "@/components/metro/LineBadge";
import { Button } from "@/components/ui/Button";
import { orgHome } from "@/features/orgs/contract";
import { MenuButton } from "@/features/shell/MenuButton";
import type { Locale } from "@/i18n/config";
import type { KnowledgeBase } from "@/lib/ragx/types";

import { deleteKnowledgeBase } from "../api";
import { ConfirmDelete } from "./ConfirmDelete";
import { RenameKnowledgeBaseDialog } from "./KnowledgeBaseDialogs";

/** The KB's platform sign (the Agent sign's band): name at display size, its plate, count and model in mono. */
export function KnowledgeBaseSign({ orgId, knowledgeBase }: { orgId: string; knowledgeBase: KnowledgeBase }) {
  const t = useTranslations("kb");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const close = (open: boolean) => !open && setDialog(null);
  return (
    <header className="bg-sidebar text-sidebar-foreground border-sidebar-border dark:border-b">
      <div className="mx-auto w-full max-w-272 px-4 pt-4 pb-6 md:px-8 md:pt-6 md:pb-7">
        <div className="flex items-center gap-2">
          <MenuButton />
          <Link
            href={orgHome(orgId)}
            className="text-muted-foreground hover:text-sidebar-foreground inline-flex min-h-11 items-center gap-1.5 text-xs font-medium transition-colors md:min-h-0"
          >
            <ArrowLeft aria-hidden className="size-3.5" />
            {t("back")}
          </Link>
        </div>
        <h1 className="animate-rise mt-4 text-[clamp(1.75rem,3vw,2.6rem)] leading-tight font-bold tracking-[-0.025em] [overflow-wrap:anywhere]">
          {knowledgeBase.name}
        </h1>
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
          <LineBadge line="kb" icon={LibraryBig}>
            {t("plate")}
          </LineBadge>
          <span className="tabular text-muted-foreground text-xs">
            {t("files", { count: knowledgeBase.fileCount })}
          </span>
          <span className="tabular text-muted-foreground text-xs">
            {knowledgeBase.embeddingModel} · {knowledgeBase.embeddingDims}
          </span>
        </div>
        <div className="mt-5 flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setDialog("rename")}>
            {t("rename")}
          </Button>
          {!knowledgeBase.isDefault && (
            <Button variant="outline" onClick={() => setDialog("delete")}>
              {t("delete")}
            </Button>
          )}
        </div>
      </div>
      {dialog === "rename" && (
        <RenameKnowledgeBaseDialog orgId={orgId} knowledgeBase={knowledgeBase} onOpenChange={close} />
      )}
      {dialog === "delete" && (
        <ConfirmDelete
          onOpenChange={close}
          title={t("deleteTitle", { name: knowledgeBase.name })}
          body={t("deleteBody")}
          onConfirm={async () => {
            await deleteKnowledgeBase(orgId, knowledgeBase.id, locale);
            router.push(orgHome(orgId));
          }}
        />
      )}
    </header>
  );
}
