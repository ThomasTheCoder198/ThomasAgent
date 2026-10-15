import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/** A placeholder block sized like the content it stands for; the pulse stops under reduced motion. */
export function Skeleton({ className, ...props }: ComponentProps<"div">) {
  return (
    <div data-slot="skeleton" className={cn("bg-muted animate-pulse rounded-sm", className)} {...props} />
  );
}
