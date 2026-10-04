import { cn } from "@/lib/cn";

import { lineBg, type Line } from "./lines";

/** The coloured dots that tell which lines a conversation or nav item touches. */
export function LineDots({ lines, size = "sm" }: { lines: readonly Line[]; size?: "sm" | "md" }) {
  if (lines.length === 0) return null;
  return (
    <span className="flex gap-0.5" aria-hidden>
      {lines.map((line) => (
        <span
          key={line}
          className={cn("rounded-full", size === "sm" ? "size-1.5" : "size-2", lineBg[line])}
        />
      ))}
    </span>
  );
}
