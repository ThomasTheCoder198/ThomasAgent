"use client";

import { useCallback, useSyncExternalStore } from "react";

/** The orb accepts only `#rgb`, `#rrggbb` or `rgb()`; anything else from a custom property is ignored. */
const COLOR_PATTERN = /^(#[0-9a-f]{3,8}|rgba?\()/i;
const THEME_ATTRIBUTES = ["data-theme", "class"];

/**
 * A theme token as a concrete colour string, for libraries that cannot read `var(--x)` (canvas orbs).
 * It watches the root element rather than the next-themes state: the provider writes `data-theme` in its own
 * effect, which runs after a child's, so a value read on the theme change would still be the old theme's.
 * Server render, jsdom and unreadable values all get `fallback`.
 */
export function useCssColor(variable: string, fallback: string): string {
  const subscribe = useCallback((notify: () => void) => {
    const observer = new MutationObserver(notify);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: THEME_ATTRIBUTES });
    return () => observer.disconnect();
  }, []);
  const read = useCallback(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue(variable).trim();
    return COLOR_PATTERN.test(value) ? value : fallback;
  }, [variable, fallback]);
  return useSyncExternalStore(subscribe, read, () => fallback);
}
