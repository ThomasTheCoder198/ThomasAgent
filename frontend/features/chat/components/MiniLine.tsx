import { Fragment } from "react";

import { lineBg, lineBorder, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

/**
 * The whole run folded into a strip: one dot per station, joined by the line it leads into.
 * `pending` adds a hollow stop for a station the run is waiting at (an approval), so a paused run never
 * reads as finished.
 */
export function MiniLine({ lines, pending }: { lines: readonly Line[]; pending?: Line }) {
  return (
    <span className="inline-flex items-center" aria-hidden>
      {lines.map((line, index) => (
        <Fragment key={`${line}-${index}`}>
          {index > 0 && <span className={cn("h-[3px] w-3", lineBg[line])} />}
          <span className={cn("size-2 rounded-full", lineBg[line])} />
        </Fragment>
      ))}
      {pending && (
        <>
          <span className={cn("h-[3px] w-3 opacity-50", lineBg[pending])} />
          <span className={cn("bg-card size-3 rounded-full border-2 border-dashed", lineBorder[pending])} />
        </>
      )}
    </span>
  );
}
