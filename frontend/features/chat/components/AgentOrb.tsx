"use client";

import { ThinkingOrb, type OrbSize, type OrbState } from "thinking-orbs";

import type { Line } from "@/components/lines";
import { useMediaQuery } from "@/hooks/useMediaQuery";

import { useCssColor } from "../useCssColor";

/** Violet for the agent itself; a line colour for the tool source it is working in. */
export type OrbTint = Line | "primary";

const TINT_VARIABLE: Record<OrbTint, string> = {
  primary: "--primary",
  // The model has no colour of its own (its line is ink); its live orb is the one violet allowed on a station.
  model: "--primary",
  kb: "--kb",
  mcp: "--mcp",
  cmp: "--cmp",
  app: "--app",
};

/** Light-theme values, used until the browser has resolved the token (and in tests). */
const TINT_FALLBACK: Record<OrbTint, string> = {
  primary: "#5558e6",
  model: "#5558e6",
  kb: "#1a7f3c",
  mcp: "#2563c9",
  cmp: "#d96f00",
  app: "#c5352b",
};

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

type Props = { state: OrbState; size: OrbSize; tint?: OrbTint; className?: string };

/** A Thinking Orb tinted from the theme tokens. It is decorative: the text beside it carries the meaning. */
export function AgentOrb({ state, size, tint = "primary", className }: Props) {
  const color = useCssColor(TINT_VARIABLE[tint], TINT_FALLBACK[tint]);
  const reducedMotion = useMediaQuery(REDUCED_MOTION);
  return (
    <ThinkingOrb
      state={state}
      size={size}
      color={color}
      paused={reducedMotion}
      aria-hidden
      className={className ?? "block"}
    />
  );
}

/** Spec §10.1: searching for KB, connecting for MCP/Composio/Apps, solving while the model thinks. */
export function orbFor(line: Line): OrbState {
  if (line === "kb") return "searching";
  if (line === "model") return "solving";
  return "connecting";
}
