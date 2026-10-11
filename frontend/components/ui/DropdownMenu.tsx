"use client";

import { DropdownMenu as MenuPrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

export const DropdownMenu = MenuPrimitive.Root;
export const DropdownMenuTrigger = MenuPrimitive.Trigger;

/** Menus that open from the sign wear the sign; the lift shadow marks them as floating. */
export function DropdownMenuContent({ className, ...props }: ComponentProps<typeof MenuPrimitive.Content>) {
  return (
    <MenuPrimitive.Portal>
      <MenuPrimitive.Content
        sideOffset={6}
        className={cn(
          "bg-sign text-sign-ink border-sign-rule shadow-lift animate-fade z-50 max-h-[min(24rem,var(--radix-dropdown-menu-content-available-height))] min-w-56 overflow-y-auto rounded-xl border p-1.5",
          className,
        )}
        {...props}
      />
    </MenuPrimitive.Portal>
  );
}

export function DropdownMenuItem({ className, ...props }: ComponentProps<typeof MenuPrimitive.Item>) {
  return (
    <MenuPrimitive.Item
      className={cn(
        "data-[highlighted]:bg-sign-active min-h-touch flex cursor-default items-center gap-2.5 rounded-md px-2.5 text-sm outline-none select-none data-[disabled]:opacity-60 md:min-h-10",
        className,
      )}
      {...props}
    />
  );
}

export function DropdownMenuSeparator({
  className,
  ...props
}: ComponentProps<typeof MenuPrimitive.Separator>) {
  return <MenuPrimitive.Separator className={cn("bg-sign-rule -mx-1.5 my-1.5 h-px", className)} {...props} />;
}

export function DropdownMenuLabel({ className, ...props }: ComponentProps<typeof MenuPrimitive.Label>) {
  return (
    <MenuPrimitive.Label
      className={cn("text-sign-ink-2 px-2.5 py-1.5 text-xs font-semibold", className)}
      {...props}
    />
  );
}
