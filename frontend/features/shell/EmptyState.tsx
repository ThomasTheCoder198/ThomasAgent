import { Inbox, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { lineIcons } from "@/components/lineIcons";
import { isToolLine, lineText, lineTint, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

/** Centred empty or status block: a muted icon in a circle, a title, one line of body and an optional action. */
export function EmptyState({
  title,
  body,
  line,
  icon,
  level = "h1",
  children,
}: {
  title: string;
  body: string;
  /** Tints the icon circle in a tool source's colour. */
  line: Line;
  /** Defaults to the line's own glyph. */
  icon?: LucideIcon;
  /** `h2` when the page already has its own `h1`. */
  level?: "h1" | "h2";
  children?: ReactNode;
}) {
  const Heading = level;
  const Icon = icon ?? (isToolLine(line) ? lineIcons[line] : Inbox);
  return (
    <section className="mx-auto flex max-w-xl flex-col items-center gap-3 px-4 py-12 text-center md:px-6 md:py-20">
      <span
        aria-hidden
        className={cn("grid size-12 place-items-center rounded-full", lineTint[line], lineText[line])}
      >
        <Icon className="size-8" strokeWidth={1.5} />
      </span>
      <Heading className="text-lg font-semibold tracking-tight">{title}</Heading>
      <p className="text-muted-foreground max-w-[56ch]">{body}</p>
      {children}
    </section>
  );
}
