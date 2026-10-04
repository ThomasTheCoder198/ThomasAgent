import { useId, type InputHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type Props = InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string };

/** Labelled input; `error` marks it invalid and links the message for assistive tech. */
export function TextField({ label, error, className, ...props }: Props) {
  const errorId = useId();
  return (
    <label className="flex flex-col gap-1.5 text-sm font-medium">
      {label}
      <input
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? errorId : undefined}
        className={cn(
          "border-rule-strong bg-paper text-ink focus-visible:border-ink rounded-lg border px-3 py-2.5 text-base font-normal transition-colors outline-none focus-visible:outline-2 disabled:opacity-70",
          error && "border-app focus-visible:border-app",
          className,
        )}
        {...props}
      />
      {error && (
        <span id={errorId} className="text-app animate-rise text-[13px] font-medium">
          {error}
        </span>
      )}
    </label>
  );
}
