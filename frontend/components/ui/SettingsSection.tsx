import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

type Props = { title: string; hint?: string; children: ReactNode; className?: string };

/** One ruled settings row: what it is on the left, the control on the right (stacked on phones). */
export function SettingsSection({ title, hint, children, className }: Props) {
  return (
    <section
      className={cn(
        "border-rule-2 grid gap-3 border-b py-6 first:pt-2 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-10",
        className,
      )}
    >
      <div>
        <h2 className="text-[15px] font-semibold">{title}</h2>
        {hint && <p className="text-ink-2 mt-1 text-[13.5px] leading-relaxed">{hint}</p>}
      </div>
      <div className="min-w-0">{children}</div>
    </section>
  );
}
