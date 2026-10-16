import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { staggerStyle } from "@/lib/stagger";

type Props = {
  title: string;
  hint?: string;
  children: ReactNode;
  className?: string;
  /** Position among the rows that enter together; later rows start a beat after earlier ones. */
  index?: number;
};

/** One settings row: what it is on the left, the control on the right (1fr : 2fr from lg, stacked on phones). */
export function SettingsSection({ title, hint, children, className, index = 0 }: Props) {
  return (
    <section
      style={staggerStyle(index)}
      className={cn(
        "animate-rise border-border grid gap-4 border-b py-6 first:pt-0 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-8",
        className,
      )}
    >
      <div>
        <h2 className="text-base font-semibold">{title}</h2>
        {hint && <p className="text-muted-foreground mt-1 text-sm leading-relaxed">{hint}</p>}
      </div>
      <div className="min-w-0">{children}</div>
    </section>
  );
}
