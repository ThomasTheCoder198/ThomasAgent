import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/** Card fill, input stroke, 4px radius; 40px tall, 44px below md. 16px type on phones so iOS does not zoom on focus. */
export function Input({ className, type, ...props }: ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "border-input bg-card text-foreground placeholder:text-muted-foreground aria-invalid:border-destructive file:text-foreground h-10 w-full min-w-0 rounded-sm border px-3 py-2 text-base transition-colors file:inline-flex file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:cursor-not-allowed disabled:opacity-60 max-md:min-h-11 md:text-[0.9375rem]",
        className,
      )}
      {...props}
    />
  );
}
