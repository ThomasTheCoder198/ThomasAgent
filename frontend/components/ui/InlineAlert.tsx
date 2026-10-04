import { TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

/** A short in-place error: what failed (title) and the server's own words (detail). Not a modal, not a toast. */
export function InlineAlert({
  title,
  detail,
  children,
}: {
  title: string;
  detail?: string;
  children?: ReactNode;
}) {
  return (
    <div role="alert" className="border-app/40 bg-app-tint flex gap-3 rounded-lg border px-3.5 py-3 text-sm">
      <TriangleAlert aria-hidden className="text-app mt-0.5 size-4 shrink-0" />
      <div className="min-w-0">
        <p className="font-semibold">{title}</p>
        {detail && <p className="text-ink-2 mt-0.5">{detail}</p>}
        {children}
      </div>
    </div>
  );
}
