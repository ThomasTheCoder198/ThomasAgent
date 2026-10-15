"use client";

import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/** A segmented choice: one quiet track, the selected segment lifts onto a card fill with a hairline. */
export function ToggleGroup({ className, ...props }: ComponentProps<typeof ToggleGroupPrimitive.Root>) {
  return (
    <ToggleGroupPrimitive.Root
      data-slot="toggle-group"
      className={cn(
        "border-border bg-secondary flex w-fit items-center gap-0.5 rounded-md border p-0.5",
        className,
      )}
      {...props}
    />
  );
}

export function ToggleGroupItem({ className, ...props }: ComponentProps<typeof ToggleGroupPrimitive.Item>) {
  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      className={cn(
        "text-muted-foreground hover:text-foreground ease-out-expo data-[state=on]:bg-card data-[state=on]:text-foreground data-[state=on]:border-border inline-flex h-8 min-w-0 flex-1 items-center justify-center gap-1.5 rounded-sm border border-transparent px-3 text-sm font-medium whitespace-nowrap transition-[color,background-color,border-color] duration-150 disabled:pointer-events-none disabled:opacity-60 data-[state=on]:font-semibold max-md:min-h-11 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    />
  );
}
