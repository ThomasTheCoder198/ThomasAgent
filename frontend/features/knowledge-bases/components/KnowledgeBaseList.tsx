import Link from "next/link";
import { getTranslations } from "next-intl/server";

import { Badge } from "@/components/ui/Badge";
import type { KnowledgeBase } from "@/lib/ragx/types";

import { kbPage } from "../contract";

/** Ruled rows, no cards: the name leads, then the file count and the embedding model as mono data. */
export async function KnowledgeBaseList({ orgId, items }: { orgId: string; items: KnowledgeBase[] }) {
  const t = await getTranslations("kb");
  return (
    <ul aria-label={t("listLabel")} className="border-border border-t text-sm">
      {items.map((kb) => (
        <li key={kb.id}>
          <Link
            href={kbPage(orgId, kb.id)}
            className="border-border hover:bg-accent grid min-h-14 items-center gap-x-6 gap-y-1 border-b px-3 py-3 transition-colors max-md:min-h-11 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,14rem)]"
          >
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[0.9375rem] font-semibold" title={kb.name}>
                {kb.name}
              </span>
              {kb.isDefault && <Badge variant="outline">{t("default")}</Badge>}
            </span>
            <span className="tabular text-muted-foreground text-xs">
              {t("files", { count: kb.fileCount })}
            </span>
            <span className="tabular text-muted-foreground truncate text-xs">
              {kb.embeddingModel} · {kb.embeddingDims}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
