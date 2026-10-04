import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { lineBg, onLine, type Line } from "@/components/metro/lines";
import { cn } from "@/lib/cn";

/** A line's name plate: the solid colour block every tool source wears (sidebar, approvals, evidence sign). */
export function LineBadge({
  line,
  icon: Icon,
  children,
}: {
  line: Line;
  icon?: LucideIcon;
  children: ReactNode;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-[5px] px-2 py-0.5 text-xs font-bold whitespace-nowrap",
        lineBg[line],
        onLine[line],
      )}
    >
      {Icon && <Icon aria-hidden className="size-3.5" />}
      {children}
    </span>
  );
}
