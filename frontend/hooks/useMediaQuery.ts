"use client";

import { useSyncExternalStore } from "react";

/** The md breakpoint in CSS px; Tailwind's `md:` starts here with the 16px root font. */
export const MD_MIN_WIDTH_PX = 768;

/** Tracks a CSS media query; `serverValue` is what the first (server and hydration) render assumes. */
export function useMediaQuery(query: string, serverValue = false): boolean {
  return useSyncExternalStore(
    (notify) => {
      const list = window.matchMedia(query);
      list.addEventListener("change", notify);
      return () => list.removeEventListener("change", notify);
    },
    () => window.matchMedia(query).matches,
    () => serverValue,
  );
}

/** True below md. Server renders assume desktop, so phones correct themselves right after hydration. */
export function useIsBelowMd(): boolean {
  return useMediaQuery(`(max-width: ${MD_MIN_WIDTH_PX - 1}px)`);
}
