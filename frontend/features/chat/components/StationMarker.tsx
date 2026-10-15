"use client";

import { lineBorder, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

import { AgentOrb, orbFor } from "./AgentOrb";

type Props = {
  line: Line;
  live?: boolean;
  tone?: "default" | "error" | "awaiting" | "muted";
};

/** A station on the run line: a roundel ringed in the line's colour, or a Thinking Orb while it is the live one. */
export function StationMarker({ line, live = false, tone = "default" }: Props) {
  if (live) {
    return (
      // The current stop outweighs every finished one: the orb sits inside a ring in the line's colour.
      <span
        className={cn("bg-card grid size-7 place-items-center rounded-full border-[3px]", lineBorder[line])}
      >
        <AgentOrb state={orbFor(line)} size={20} tint={line} />
      </span>
    );
  }
  return (
    <span
      className={cn(
        // A dashed ring is a stop the run has not reached yet (waiting for approval).
        "bg-card block size-5 rounded-full border-[3px]",
        tone === "awaiting" && "border-dashed",
        tone === "error" ? "border-app" : tone === "muted" ? "border-border" : lineBorder[line],
      )}
    />
  );
}
