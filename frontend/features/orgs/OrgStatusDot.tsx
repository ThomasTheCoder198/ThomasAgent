import { cn } from "@/lib/cn";

import type { RagxStatus } from "./contract";

/** On the sign: solid = ready, hollow ring = preparing, Apps red = failed. Green stays the KB line's. */
export function OrgStatusDot({ status, label }: { status: RagxStatus; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      className={cn(
        "inline-block size-2.5 shrink-0 rounded-full",
        status === "ready" && "bg-sidebar-foreground",
        status === "pending" && "border-muted-foreground border-2",
        status === "failed" && "bg-app",
      )}
    />
  );
}
