import type { InputHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type Props = InputHTMLAttributes<HTMLInputElement> & { label: string };

export function TextField({ label, className, ...props }: Props) {
  return (
    <label className="flex flex-col gap-1.5 text-sm font-medium">
      {label}
      <input
        className={cn(
          "border-rule-strong bg-paper text-ink focus-visible:border-ink rounded-lg border px-3 py-2.5 text-base font-normal transition-colors outline-none focus-visible:outline-2",
          className,
        )}
        {...props}
      />
    </label>
  );
}
