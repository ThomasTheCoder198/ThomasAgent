import { Fragment } from "react";

import { lineBg, lineBorder, type Line } from "@/components/metro/lines";
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
          {index > 0 && <span className={cn("h-[3px] w-3.5", lineBg[line])} />}
          <span className={cn("size-[9px] rounded-full", lineBg[line])} />
        </Fragment>
      ))}
      {pending && (
        <>
          <span className={cn("h-[3px] w-3.5 opacity-50", lineBg[pending])} />
          <span className={cn("bg-paper size-[11px] rounded-full border-2", lineBorder[pending])} />
        </>
      )}
    </span>
  );
}
