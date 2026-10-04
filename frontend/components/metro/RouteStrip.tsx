import { cn } from "@/lib/cn";

import { lineBg, lineBorder, type Line } from "./lines";

/** `short` replaces `label` on phones, where five stops share 340px. */
export type RouteStop = { line: Line; label: string; short?: string };

// The strip always sits on the enamel-black sign, where the model line's theme ink would vanish.
const stopBorder: Record<Line, string> = { ...lineBorder, model: "border-sign-ink" };
const segmentBg: Record<Line, string> = { ...lineBg, model: "bg-sign-ink" };

/**
 * A horizontal line map: each stop is a roundel on its line's colour, joined by the line it leads into.
 * Decorative when `labelled` is false; the labels are real text otherwise.
 */
export function RouteStrip({ stops, labelled = true }: { stops: readonly RouteStop[]; labelled?: boolean }) {
  return (
    <ol className="flex w-full items-start" aria-hidden={!labelled}>
      {stops.map(({ line, label, short }, index) => {
        const next = stops[index + 1];
        return (
          <li key={label} className="flex min-w-0 flex-1 flex-col gap-2.5 last:flex-none">
            <span className="flex items-center">
              <span className={cn("bg-sign size-4.5 shrink-0 rounded-full border-[4px]", stopBorder[line])} />
              {next && <span className={cn("h-1.5 flex-1", segmentBg[next.line])} />}
            </span>
            {labelled && (
              <span className="text-sign-ink-2 truncate pr-2 text-xs font-medium">
                <span className="sm:hidden">{short ?? label}</span>
                <span className="hidden sm:inline">{label}</span>
              </span>
            )}
          </li>
        );
      })}
    </ol>
  );
}
