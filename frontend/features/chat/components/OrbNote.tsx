"use client";

import type { OrbSize, OrbState } from "thinking-orbs";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { AgentOrb } from "./AgentOrb";

type Props = {
  state: OrbState;
  size: OrbSize;
  children: ReactNode;
  /** Announce the text politely: for a state change the owner would otherwise miss. */
  live?: boolean;
  className?: string;
};

/** An orb with one muted line beside it: the agent is working, and this says on what. */
export function OrbNote({ state, size, children, live = false, className }: Props) {
  return (
    <div
      role={live ? "status" : undefined}
      aria-live={live ? "polite" : undefined}
      className={cn("text-muted-foreground flex items-center gap-3 text-sm", className)}
    >
      <AgentOrb state={state} size={size} />
      <span>{children}</span>
    </div>
  );
}
