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
    <ul className={cn("flex flex-col gap-0.5", compact ? "px-3.5 pt-2.5 pb-3" : "p-3")}>
      {citations.map((c) => (
        <li key={c.n}>
          <button
            type="button"
            onClick={() => onSelect(c.n)}
            aria-current={c.n === selected ? "true" : undefined}
            className="hover:bg-ground aria-[current=true]:bg-kb-tint grid w-full grid-cols-[auto_1fr_auto] items-center gap-2.5 rounded-md px-1.5 py-1.5 text-left text-[13px]"
          >
            <CitationRoundel n={c.n} label={tChat("citation", { n: c.n })} />
            <span className="min-w-0 truncate">
              {c.fileName}
              {c.locator && (
                <small className="text-ink-3 ml-1.5 text-xs">
                  {t("pageShort", { page: c.locator.page })} {c.title}
                </small>
              )}
            </span>
            {c.locator && (
              <span className="tabular text-ink-3 text-[11px]" title={t("score")}>
                {c.locator.score.toFixed(SCORE_DIGITS)}
              </span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}
