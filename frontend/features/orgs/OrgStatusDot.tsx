import { cn } from "@/lib/cn";

import type { RagxStatus } from "./contract";

/** Solid green = ready, hollow ring = preparing, destructive red = failed. */
export function OrgStatusDot({ status, label }: { status: RagxStatus; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      className={cn(
        "inline-block size-2.5 shrink-0 rounded-full",
        status === "ready" && "bg-success",
        status === "pending" && "border-muted-foreground border-2",
        status === "failed" && "bg-destructive",
      )}
    />
  );
}
