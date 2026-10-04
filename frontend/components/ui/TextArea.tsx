import type { TextareaHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type Props = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  label: string;
  hideLabel?: boolean;
  footer?: string;
};

/** Multiline field that grows with its content; `footer` carries a counter or hint under the box. */
export function TextArea({ label, hideLabel, footer, className, ...props }: Props) {
  return (
    <label className="flex flex-col gap-1.5 text-sm font-medium">
      <span className={cn(hideLabel && "sr-only")}>{label}</span>
      <textarea
        className={cn(
          "border-rule-strong bg-paper text-ink focus-visible:border-ink block field-sizing-content w-full resize-y rounded-lg border px-3 py-2.5 text-[15px] leading-relaxed font-normal transition-colors outline-none focus-visible:outline-none",
          className,
        )}
        {...props}
      />
      {footer && <span className="tabular text-ink-3 self-end text-xs font-normal">{footer}</span>}
    </label>
  );
}
