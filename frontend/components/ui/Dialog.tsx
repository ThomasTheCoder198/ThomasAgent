"use client";

import { XIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { Dialog as DialogPrimitive } from "radix-ui";
import type { ComponentProps, RefObject } from "react";

import { cn } from "@/lib/cn";

import { useModalLayer } from "./useModalLayer";

export function Dialog(props: ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />;
}

export function DialogTrigger(props: ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />;
}

export function DialogClose(props: ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

export function DialogOverlay({ className, ...props }: ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "bg-scrim data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 fixed inset-0 z-40 duration-200",
        className,
      )}
      {...props}
    />
  );
}

type ContentProps = ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean;
  /** Where focus goes on close when the opener unmounts before the dialog does (a menu item, for one). */
  returnFocusRef?: RefObject<HTMLElement | null>;
};

/** A lifted notice on a scrim: card fill, lift shadow, 14px radius; focus is trapped and returned. */
export function DialogContent({
  className,
  children,
  showCloseButton = true,
  returnFocusRef,
  ...props
}: ContentProps) {
  const t = useTranslations("common");
  const layer = useModalLayer(returnFocusRef);
  return (
    <DialogPrimitive.Portal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        {...props}
        {...layer}
        className={cn(
          "bg-card text-card-foreground border-border shadow-lift data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-lg border p-5 duration-200 sm:p-6",
          className,
        )}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            className="text-muted-foreground hover:bg-accent hover:text-foreground absolute top-3 right-3 grid size-8 place-items-center rounded-md transition-colors max-md:size-11 [&_svg]:size-4"
          >
            <XIcon aria-hidden />
            <span className="sr-only">{t("close")}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}

export function DialogHeader({ className, ...props }: ComponentProps<"div">) {
  return <div data-slot="dialog-header" className={cn("flex flex-col gap-1.5 pr-8", className)} {...props} />;
}

/**
 * Title and optional description. Radix expects a description for the dialog's accessible description; without
 * one, the title doubles as a screen-reader-only description so the dialog still announces its purpose.
 */
export function DialogHeading({ title, description }: { title: string; description?: string }) {
  return (
    <DialogHeader>
      <DialogTitle>{title}</DialogTitle>
      <DialogDescription className={cn(!description && "sr-only")}>{description ?? title}</DialogDescription>
    </DialogHeader>
  );
}

export function DialogFooter({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    />
  );
}

export function DialogTitle({ className, ...props }: ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("text-lg leading-snug font-semibold tracking-tight", className)}
      {...props}
    />
  );
}

export function DialogDescription({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn("text-muted-foreground text-sm", className)}
      {...props}
    />
  );
}
