"use client";

import { useTranslations } from "next-intl";

import { cn } from "@/lib/cn";

import { CitationRoundel } from "../components/CitationRoundel";
import type { Citation } from "../run-view";

const SCORE_DIGITS = 2;

type Props = {
  citations: Citation[];
  selected: number | undefined;
  onSelect: (n: number) => void;
  compact?: boolean;
};

/** Every source the answer cited, in roundel order, with page, section and match score. */
export function SourceList({ citations, selected, onSelect, compact }: Props) {
  const t = useTranslations("evidence");
  const tChat = useTranslations("chat");
  return (
    <ul className={cn("flex flex-col gap-1", compact ? "px-3 py-2" : "p-3")}>
      {citations.map((c) => (
        <li key={c.n}>
          <button
            type="button"
            onClick={() => onSelect(c.n)}
            aria-current={c.n === selected ? "true" : undefined}
            className="hover:bg-accent aria-[current=true]:bg-accent grid min-h-10 w-full grid-cols-[auto_1fr_auto] items-center gap-3 rounded-md px-2 py-2 text-left text-sm max-md:min-h-11"
          >
            <CitationRoundel n={c.n} label={tChat("citation", { n: c.n })} />
            <span className="min-w-0 truncate">
              {c.fileName}
              {c.locator && (
                <small className="text-muted-foreground ml-2 text-xs">
                  {t("pageShort", { page: c.locator.page })} {c.title}
                </small>
              )}
            </span>
            {c.locator && (
              <span className="tabular text-muted-foreground text-xs" title={t("score")}>
                {c.locator.score.toFixed(SCORE_DIGITS)}
              </span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}
