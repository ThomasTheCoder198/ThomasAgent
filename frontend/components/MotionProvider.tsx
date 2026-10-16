"use client";

import { MotionConfig } from "motion/react";
import type { ReactNode } from "react";

import { useMediaQuery } from "@/hooks/useMediaQuery";

/**
 * A snappy spring for hover and press feedback. `reducedMotion="never"` is deliberate (owner decision
 * 2026-10-16): with the OS's animation effects off the app keeps its gentle micro-interactions, and only large
 * travel is withheld, through `useGentleMotion`.
 */
const SPRING = { type: "spring", stiffness: 500, damping: 32, mass: 0.6 } as const;

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

/** Orb speed multiplier when the OS asks for less motion: still visibly alive, just slower. */
const REDUCED_ORB_SPEED = 0.6;

export function MotionProvider({ children }: { children: ReactNode }) {
  return (
    <MotionConfig reducedMotion="never" transition={SPRING}>
      {children}
    </MotionConfig>
  );
}

/**
 * What the user's motion preference changes. Entrances are CSS (`animate-rise` reads `--rise-y`, which reduced
 * motion zeroes), because they must render from the server without a hydration mismatch; JS only needs the
 * orbs' speed, which stays alive but calmer.
 */
export function useGentleMotion() {
  // Not motion's own `useReducedMotion`: it logs a dev warning whenever the OS preference is on, which is the owner's case.
  const reduced = useMediaQuery(REDUCED_MOTION_QUERY);
  return { reduced, orbSpeed: reduced ? REDUCED_ORB_SPEED : 1 };
}
