import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { lineBg, onLine, type Line } from "@/components/lines";
import { Badge } from "@/components/ui/Badge";
import { cn } from "@/lib/cn";

/** A tool source's name plate: the line colour is the data, so it fills the pill. */
export function SourceBadge({
  line,
  icon: Icon,
  children,
}: {
  line: Line;
  icon?: LucideIcon;
  children: ReactNode;
}) {
  return (
    <Badge className={cn(lineBg[line], onLine[line])}>
      {Icon && <Icon aria-hidden />}
      {children}
    </Badge>
  );
}
