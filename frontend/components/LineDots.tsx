import { lineBg, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

/** Small dots in the colours of the tool sources a conversation or nav item touches. */
export function LineDots({ lines, size = "sm" }: { lines: readonly Line[]; size?: "sm" | "md" }) {
  if (lines.length === 0) return null;
  return (
    <span className="flex gap-1" aria-hidden>
      {lines.map((line) => (
        <span
          key={line}
          className={cn("rounded-full", size === "sm" ? "size-1.5" : "size-2", lineBg[line])}
        />
      ))}
    </span>
  );
}
