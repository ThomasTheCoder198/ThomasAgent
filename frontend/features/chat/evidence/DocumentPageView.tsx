"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef } from "react";

import { cn } from "@/lib/cn";

import type { DocumentPage } from "../contract";
import type { Citation } from "../run-view";
import { CitationRoundel } from "../components/CitationRoundel";

type Props = {
  page: DocumentPage;
  citations: Citation[];
  selected: number | undefined;
  onSelect: (n: number) => void;
};

/**
 * The cited page drawn block by block. M1 swaps the paragraphs for the PDF render with a bbox overlay;
 * the highlight contract (block ids from the citation locator) stays the same.
 */
export function DocumentPageView({ page, citations, selected, onSelect }: Props) {
  const t = useTranslations("chat");
  const hitRef = useRef<HTMLParagraphElement>(null);
  const onPage = citations.filter(
    (c) => c.locator?.documentId === page.documentId && c.locator.page === page.page,
  );
  const selectedBlocks = new Set(onPage.find((c) => c.n === selected)?.locator?.blockIds ?? []);

  useEffect(() => {
    hitRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [selected]);

  return (
    <div className="bg-page text-page-ink shadow-page relative rounded px-6.5 pt-6.5 pb-7.5 text-[12.5px] leading-relaxed">
      <span className="tabular absolute top-3 right-4 text-[10px] opacity-70">{page.page}</span>
      {page.blocks.map((block) => {
        const tags = onPage.filter((c) => c.locator?.blockIds.includes(block.id));
        const hit = selectedBlocks.has(block.id);
        if (block.kind === "heading") {
          return (
            <h3 key={block.id} className="mt-0.5 mb-2 font-bold">
              {block.text}
            </h3>
          );
        }
        return (
          <p
            key={block.id}
            ref={hit ? hitRef : undefined}
            className={cn(
              "outline-kb relative mb-2 rounded-[3px] transition-[background-color,outline-color] duration-300",
              hit && "bg-page-hit animate-hit outline-2",
            )}
          >
            {tags.length > 0 && (
              <span className="absolute top-px -left-6.5 flex flex-col gap-0.5">
                {tags.map((c) => (
                  <CitationRoundel
                    key={c.n}
                    n={c.n}
                    label={t("citation", { n: c.n })}
                    selected={c.n === selected}
                    onSelect={onSelect}
                    className="mx-0"
                  />
                ))}
              </span>
            )}
            {block.text}
          </p>
        );
      })}
    </div>
  );
}
