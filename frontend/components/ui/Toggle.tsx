"use client";

import { cva, type VariantProps } from "class-variance-authority";
import { Toggle as TogglePrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

export const toggleVariants = cva(
  "hover:bg-accent data-[state=on]:bg-accent data-[state=on]:text-accent-foreground ease-out-expo inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-[color,background-color] duration-150 disabled:pointer-events-none disabled:opacity-60 max-md:min-h-11 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        outline: "border-input border bg-transparent",
      },
      size: {
        sm: "h-8 min-w-8 px-2",
        default: "h-9 min-w-9 px-3",
        lg: "h-11 min-w-11 px-4",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export function Toggle({
  className,
  variant,
  size,
  ...props
}: ComponentProps<typeof TogglePrimitive.Root> & VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive.Root
      data-slot="toggle"
      className={cn(toggleVariants({ variant, size }), className)}
      {...props}
    />
  );
}
