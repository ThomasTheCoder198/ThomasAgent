"use client";

import { useTranslations } from "next-intl";
import { Dialog as SheetPrimitive } from "radix-ui";
import type { ComponentProps, RefObject } from "react";

import { cn } from "@/lib/cn";

import { OsTitlebar, OsWindow, OsWindowBody, OsWindowClose } from "./OsWindow";
import { useModalLayer } from "./useModalLayer";

export function Sheet(props: ComponentProps<typeof SheetPrimitive.Root>) {
  return <SheetPrimitive.Root data-slot="sheet" {...props} />;
}

export function SheetTrigger(props: ComponentProps<typeof SheetPrimitive.Trigger>) {
  return <SheetPrimitive.Trigger data-slot="sheet-trigger" {...props} />;
}

export function SheetClose(props: ComponentProps<typeof SheetPrimitive.Close>) {
  return <SheetPrimitive.Close data-slot="sheet-close" {...props} />;
}

export function SheetOverlay({ className, ...props }: ComponentProps<typeof SheetPrimitive.Overlay>) {
  return (
    <SheetPrimitive.Overlay
      data-slot="sheet-overlay"
      className={cn(
        "os-backdrop data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 fixed inset-0 z-40 duration-200",
        className,
      )}
      {...props}
    />
  );
}

/** A bottom sheet rounds only its top corners (18px). */
const SIDE_CLASSES = {
  left: "os-sheet-edge inset-y-0 left-0 h-full w-72 max-w-[85vw] data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left",
  right:
    "os-sheet-edge inset-y-0 right-0 h-full w-96 max-w-[85vw] data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right",
  top: "os-sheet-edge inset-x-0 top-0 h-auto data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top",
  bottom:
    "os-sheet-bottom inset-x-0 bottom-0 h-auto max-h-[85dvh] data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom",
} as const;

export type SheetSide = keyof typeof SIDE_CLASSES;

type ContentProps = ComponentProps<typeof SheetPrimitive.Content> & {
  side?: SheetSide;
  showCloseButton?: boolean;
  returnFocusRef?: RefObject<HTMLElement | null>;
};

export function SheetContent({
  className,
  children,
  side = "right",
  showCloseButton = true,
  returnFocusRef,
  ...props
}: ContentProps) {
  const t = useTranslations("common");
  const layer = useModalLayer(returnFocusRef);
  return (
    <SheetPrimitive.Portal>
      <SheetOverlay />
      <SheetPrimitive.Content
        data-slot="sheet-content"
        {...props}
        {...layer}
        className={cn(
          "data-[state=closed]:animate-out data-[state=open]:animate-in fixed z-50 flex flex-col ease-in-out data-[state=closed]:duration-300 data-[state=open]:duration-300",
          SIDE_CLASSES[side],
          className,
        )}
      >
        <OsWindow>
          <OsTitlebar
            closeControl={
              showCloseButton ? (
                <SheetPrimitive.Close asChild>
                  <OsWindowClose aria-label={t("close")} />
                </SheetPrimitive.Close>
              ) : undefined
            }
          />
          <OsWindowBody className="flex flex-col gap-4">{children}</OsWindowBody>
        </OsWindow>
      </SheetPrimitive.Content>
    </SheetPrimitive.Portal>
  );
}

export function SheetHeader({ className, ...props }: ComponentProps<"div">) {
  return (
    <div data-slot="sheet-header" className={cn("flex flex-col gap-1.5 p-4 pr-14", className)} {...props} />
  );
}

export function SheetFooter({ className, ...props }: ComponentProps<"div">) {
  return (
    <div data-slot="sheet-footer" className={cn("mt-auto flex flex-col gap-2 p-4", className)} {...props} />
  );
}

export function SheetTitle({ className, ...props }: ComponentProps<typeof SheetPrimitive.Title>) {
  return (
    <SheetPrimitive.Title
      data-slot="sheet-title"
      className={cn("text-foreground text-lg leading-snug font-semibold tracking-tight", className)}
      {...props}
    />
  );
}

export function SheetDescription({ className, ...props }: ComponentProps<typeof SheetPrimitive.Description>) {
  return (
    <SheetPrimitive.Description
      data-slot="sheet-description"
      className={cn("text-muted-foreground text-sm", className)}
      {...props}
    />
  );
}
