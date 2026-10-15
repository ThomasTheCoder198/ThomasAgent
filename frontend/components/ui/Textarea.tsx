import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/** Same skin as Input; grows with its content. */
export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "border-input bg-card text-foreground placeholder:text-muted-foreground aria-invalid:border-destructive block field-sizing-content min-h-20 w-full rounded-sm border px-3 py-2 text-base leading-relaxed transition-colors disabled:cursor-not-allowed disabled:opacity-60 md:text-[0.9375rem]",
        className,
      )}
      {...props}
    />
  );
}
