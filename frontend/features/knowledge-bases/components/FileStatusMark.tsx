"use client";

import { useTranslations } from "next-intl";

import { cn } from "@/lib/cn";
import type { FileStatus } from "@/lib/ragx/types";

/** Station vocabulary: green ring = ready (also a local-parse fallback), Apps red = failed, hollow ring = on its way. */
const RING: Record<FileStatus, string> = {
  pending: "border-border border-2",
  parsing: "border-border border-2",
  enriching: "border-border border-2",
  chunking: "border-border border-2",
  embedding: "border-border border-2",
  indexing: "border-border border-2",
  ready: "border-kb border-4",
  degraded: "border-kb border-4",
  failed: "border-app border-4",
};

export function FileStatusMark({ status, errorCode }: { status: FileStatus; errorCode: string }) {
  const t = useTranslations("kb.status");
  const label = t(status);
  return (
    <span className="inline-flex min-w-0 items-center gap-2 text-[13px]">
      <span
        role="img"
        aria-label={label}
        className={cn("bg-card block size-4.5 shrink-0 rounded-full", RING[status])}
      />
      <span className="truncate">{label}</span>
      {status === "failed" && errorCode && (
        <span className="tabular text-destructive text-xs">{errorCode}</span>
      )}
    </span>
  );
}
