import Link from "next/link";
import { getTranslations } from "next-intl/server";

import type { KnowledgeBase } from "@/lib/ragx/types";

import { kbPage } from "../contract";

/** A departure board of Knowledge Bases: one ruled row each, a green line dot, no cards. */
export async function KnowledgeBaseList({ orgId, items }: { orgId: string; items: KnowledgeBase[] }) {
  const t = await getTranslations("kb");
  return (
    <ul aria-label={t("listLabel")} className="text-sm">
      {items.map((kb, index) => (
        <li key={kb.id}>
          <Link
            href={kbPage(orgId, kb.id)}
            style={{ animationDelay: `calc(${index} * var(--stagger))` }}
            className="animate-rise border-border hover:bg-card grid min-h-14 items-center gap-x-5 gap-y-1 border-b px-3 py-3.5 transition-colors sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,14rem)]"
          >
            <span className="flex min-w-0 items-center gap-2.5">
              <span aria-hidden className="bg-kb size-2.5 shrink-0 rounded-full" />
              <span className="truncate text-[15px] font-semibold" title={kb.name}>
                {kb.name}
              </span>
              {kb.isDefault && (
                <span className="border-kb text-kb shrink-0 rounded-[5px] border px-1.5 py-px text-xs font-bold">
                  {t("default")}
                </span>
              )}
            </span>
            <span className="tabular text-muted-foreground pl-5 text-xs sm:pl-0">
              {t("files", { count: kb.fileCount })}
            </span>
            <span className="tabular text-muted-foreground truncate pl-5 text-xs sm:pl-0">
              {kb.embeddingModel} · {kb.embeddingDims}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
