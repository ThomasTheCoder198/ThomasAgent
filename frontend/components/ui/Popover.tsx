"use client";

import { Popover as PopoverPrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

const POPOVER_OFFSET = 4;

export function Popover(props: ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />;
}

export function PopoverTrigger(props: ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />;
}

/** A floating layer: card fill, hairline, lift shadow, 14px radius (DESIGN.md Elevation). */
export function PopoverContent({
  className,
  align = "start",
  sideOffset = POPOVER_OFFSET,
  ...props
}: ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        collisionPadding={16}
        className={cn(
          "bg-popover text-popover-foreground border-border shadow-lift data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 z-50 origin-(--radix-popover-content-transform-origin) overflow-hidden rounded-lg border outline-hidden duration-150",
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  );
}
