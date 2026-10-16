"use client";

import { useTranslations } from "next-intl";

import { Orb } from "@/components/Orb";
import { Badge } from "@/components/ui/Badge";
import type { FileStatus } from "@/lib/ragx/types";

import { INLINE_ORB_SIZE } from "../layout";

/** Steps where ragx is actively working on the file: this is agent work, so the badge carries an orb. */
const WORKING: ReadonlySet<FileStatus> = new Set([
  "parsing",
  "enriching",
  "chunking",
  "embedding",
  "indexing",
]);

/**
 * Ready (also a local-parse fallback, with its note beside the badge) is the success tint, failed is the
 * destructive tint with the error code in mono, and anything on its way is the quiet secondary badge.
 */
export function FileStatusBadge({ status, errorCode }: { status: FileStatus; errorCode: string }) {
  const t = useTranslations("kb");
  if (status === "ready" || status === "degraded") {
    return (
      <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant="success">{t("status.ready")}</Badge>
        {status === "degraded" && <span className="text-muted-foreground text-xs">{t("degradedNote")}</span>}
      </span>
    );
  }
  if (status === "failed") {
    return (
      <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant="destructive">{t("status.failed")}</Badge>
        {errorCode && <span className="tabular text-destructive text-xs">{errorCode}</span>}
      </span>
    );
  }
  return (
    <Badge className={WORKING.has(status) ? "pl-1" : undefined}>
      {WORKING.has(status) && <Orb state="working" size={INLINE_ORB_SIZE} />}
      {t(`status.${status}`)}
    </Badge>
  );
}
