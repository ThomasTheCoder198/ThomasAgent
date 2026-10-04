"use client";

import { useEffect, type RefObject } from "react";

/** Closes a popover, drawer or sheet on Escape and on a pointer press outside `ref`. */
export function useDismiss(open: boolean, onClose: () => void, ref?: RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const onPointer = (event: PointerEvent) => {
      if (ref?.current && !ref.current.contains(event.target as Node)) onClose();
    };
    document.addEventListener("keydown", onKey);
    if (ref) document.addEventListener("pointerdown", onPointer);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointer);
    };
  }, [open, onClose, ref]);
}
