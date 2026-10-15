import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { lineBg, onLine, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

/** A tool source's name pill, filled in its line colour (the pair in `lines.ts` is contrast-checked). */
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
        "inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2.5 text-xs font-medium whitespace-nowrap",
        lineBg[line],
        onLine[line],
      )}
    >
      {Icon && <Icon aria-hidden className="size-3.5" />}
      {children}
    </span>
  );
}
