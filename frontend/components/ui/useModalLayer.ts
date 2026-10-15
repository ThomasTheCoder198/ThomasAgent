"use client";

import { useRef, type RefObject } from "react";

const FALLBACK_FOCUS_SELECTOR = "main";
/** Sonner's toast stack; it floats above modal layers, so a press or focus there is not "outside". */
const TOAST_REGION_SELECTOR = "[data-sonner-toaster]";

/** True when an event target is inside the toast stack; modal layers must not close for those interactions. */
export function isInToastRegion(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(TOAST_REGION_SELECTOR) !== null;
}

/**
 * Layers mount already open, without a Radix Trigger, so Radix has no trigger to return focus to and focus
 * would drop to <body>. The opener is remembered when the layer takes focus; if the opener is gone (a deleted
 * row, a closed menu) focus goes to the page's main landmark instead of being lost.
 */
function restoreFocus(opener: HTMLElement | null) {
  if (opener?.isConnected) {
    opener.focus();
    return;
  }
  const fallback = document.querySelector<HTMLElement>(FALLBACK_FOCUS_SELECTOR);
  fallback?.setAttribute("tabindex", "-1");
  fallback?.focus({ preventScroll: true });
}

/** Handlers for a Radix modal Content (Dialog, AlertDialog, Sheet): focus return and toast interaction. */
export function useModalLayer(returnFocusRef?: RefObject<HTMLElement | null>) {
  const opener = useRef<HTMLElement | null>(null);
  return {
    onOpenAutoFocus: () => {
      const active = document.activeElement;
      opener.current = active instanceof HTMLElement && active !== document.body ? active : null;
    },
    onInteractOutside: (event: Event) => {
      if (isInToastRegion(event.target)) event.preventDefault();
    },
    onCloseAutoFocus: (event: Event) => {
      event.preventDefault();
      restoreFocus(returnFocusRef?.current ?? opener.current);
    },
  };
}
