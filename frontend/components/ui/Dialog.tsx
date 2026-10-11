"use client";

import { Dialog as DialogPrimitive } from "radix-ui";
import { useRef, type ReactNode, type RefObject } from "react";

import { cn } from "@/lib/cn";

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;

const FALLBACK_FOCUS_SELECTOR = "main";

/**
 * Dialogs mount already open, without a Radix Trigger, so Radix has no trigger to return focus to and
 * focus would drop to <body>. The opener is remembered when the dialog takes focus; if the opener is gone
 * (a deleted row, a closed menu) focus goes to the page's main landmark instead of being lost.
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

/** A lifted notice: paper on a scrim, title first, focus trapped and returned by Radix. */
export function DialogContent({
  title,
  description,
  children,
  className,
  returnFocusRef,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
  /** Where focus goes on close when the opener unmounts before the dialog does (a menu item, for one). */
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const opener = useRef<HTMLElement | null>(null);
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="bg-scrim animate-fade fixed inset-0 z-40" />
      <DialogPrimitive.Content
        onOpenAutoFocus={() => {
          const active = document.activeElement;
          opener.current = active instanceof HTMLElement && active !== document.body ? active : null;
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          restoreFocus(returnFocusRef?.current ?? opener.current);
        }}
        className={cn(
          "bg-paper border-rule shadow-lift animate-rise fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-5 sm:p-6",
          className,
        )}
      >
        <DialogPrimitive.Title className="text-[17px] leading-snug font-bold tracking-[-0.01em]">
          {title}
        </DialogPrimitive.Title>
        {description ? (
          <DialogPrimitive.Description className="text-ink-2 mt-1.5 text-[13.5px] leading-relaxed">
            {description}
          </DialogPrimitive.Description>
        ) : (
          <DialogPrimitive.Description className="sr-only">{title}</DialogPrimitive.Description>
        )}
        <div className="mt-5">{children}</div>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}
