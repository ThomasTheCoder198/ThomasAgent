import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/** Status variants keep foreground text on a soft tint and name the state with a coloured dot (text contrast stays AA). */
const DOT = "before:size-1.5 before:shrink-0 before:rounded-full before:content-['']";

export const badgeVariants = cva(
  "inline-flex h-6 w-fit shrink-0 items-center justify-center gap-1.5 overflow-hidden rounded-full border border-transparent px-2.5 text-xs font-medium whitespace-nowrap [&>svg]:pointer-events-none [&>svg]:size-3",
  {
    variants: {
      variant: {
        default: "bg-secondary text-secondary-foreground",
        primary: "bg-accent text-accent-foreground",
        outline: "border-border text-foreground",
        success: `bg-success/15 text-foreground before:bg-success ${DOT}`,
        warning: `bg-warning/25 text-foreground before:bg-warning ${DOT}`,
        destructive: `bg-destructive/15 text-foreground before:bg-destructive ${DOT}`,
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export function Badge({
  className,
  variant,
  asChild = false,
  ...props
}: ComponentProps<"span"> & VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span";
  return (
    <Comp
      data-slot="badge"
      data-variant={variant ?? "default"}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  );
}
