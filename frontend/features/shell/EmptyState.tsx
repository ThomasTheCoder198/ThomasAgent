import type { ReactNode } from "react";

import { lineBg, lineBorder, type Line } from "@/components/metro/lines";
import { cn } from "@/lib/cn";

/** One station on its line, not a card: the roundel heads a short stem that leads into the copy. */
export function EmptyState({
  title,
  body,
  line,
  level = "h1",
  children,
}: {
  title: string;
  body: string;
  line: Line;
  /** `h2` when the page already has its own `h1` (a sign band above the station). */
  level?: "h1" | "h2";
  children?: ReactNode;
}) {
  const Heading = level;
  return (
    <section className="mx-auto flex max-w-[68ch] items-start gap-5 px-6 py-14 md:px-8 md:py-20">
      <span className="flex flex-col items-center pt-1" aria-hidden>
        <span className={cn("bg-paper size-5.5 rounded-full border-[5px]", lineBorder[line])} />
        <span className={cn("animate-draw h-20 w-[3px] rounded-b-sm", lineBg[line])} />
      </span>
      <div>
        <Heading className="text-[22px] font-bold tracking-[-0.015em]">{title}</Heading>
        <p className="text-ink-2 mt-2 leading-relaxed">{body}</p>
        {children}
      </div>
    </section>
  );
}
