"use client";

import { ThinkingOrb } from "thinking-orbs";

import { lineBorder, lineTint, type Line } from "@/components/metro/lines";
import { cn } from "@/lib/cn";

export type OrbState = "solving" | "searching" | "connecting" | "composing";

const ORB_SIZE = 20;

/** Spec §10.1: searching for KB, connecting for MCP/Composio/Apps, solving while thinking, composing while writing. */
export function orbFor(line: Line): OrbState {
  if (line === "kb") return "searching";
  if (line === "model") return "solving";
  return "connecting";
}

type Props = {
  line: Line;
  live?: boolean;
  orb?: OrbState;
  tone?: "default" | "error" | "awaiting" | "muted";
};

/** A station on the run line: a roundel in the line's colour, or a Thinking Orb while it is the live station. */
export function StationMarker({ line, live = false, orb, tone = "default" }: Props) {
  if (live) {
    return (
      // The current stop outweighs every finished one: a thick ring in the line's colour around the orb.
      <span
        className={cn(
          "grid size-7 place-items-center rounded-full border-[3px]",
          lineBorder[line],
          lineTint[line],
        )}
      >
        <ThinkingOrb state={orb ?? orbFor(line)} size={ORB_SIZE} />
      </span>
    );
  }
  return (
    <span
      className={cn(
        // "Awaiting" is a thin hollow ring everywhere (here and in the mini line): the train has not reached it.
        "bg-paper block size-4.5 rounded-full",
        tone === "awaiting" ? "border-2" : "border-4",
        tone === "error" ? "border-app" : tone === "muted" ? "border-todo" : lineBorder[line],
      )}
    />
  );
}
