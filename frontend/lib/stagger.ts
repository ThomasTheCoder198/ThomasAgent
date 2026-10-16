import type { CSSProperties } from "react";

/** Delay step between rows that enter together, and the row after which later ones arrive with the last. */
export const STAGGER_MS = 45;
export const STAGGER_CAP = 10;

/**
 * Inline delay for a staggered `animate-rise`. It is CSS rather than Motion because lists render on the server
 * and an entrance must not depend on hydration; the cap keeps a long list from taking seconds to settle.
 */
export function staggerStyle(index: number, stepMs = STAGGER_MS): CSSProperties {
  return { animationDelay: `${Math.min(index, STAGGER_CAP) * stepMs}ms` };
}
