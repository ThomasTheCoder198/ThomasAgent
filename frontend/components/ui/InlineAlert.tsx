import { TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

/**
 * A short in-place error: what failed (title) and the server's own words (detail). Destructive text and an
 * icon, no box; not a modal and not a toast (a toast announces the same failure once).
 */
export function InlineAlert({
  title,
  detail,
  children,
}: {
  title: ReactNode;
  detail?: string;
  children?: ReactNode;
}) {
  return (
    <div role="alert" className="flex gap-2 text-sm">
      <TriangleAlert aria-hidden className="text-destructive mt-0.5 size-4 shrink-0" />
      <div className="min-w-0">
        <p className="text-destructive font-medium">{title}</p>
        {detail && <p className="text-muted-foreground mt-0.5">{detail}</p>}
        {children}
      </div>
    </div>
  );
}
