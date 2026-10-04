"use client";

import { useTranslations } from "next-intl";
import { memo, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { CITE_TAG, remarkCitations } from "../remark-citations";
import { CitationRoundel } from "./CitationRoundel";
import { Caret } from "./StationContent";

type Props = {
  text: string;
  streaming: boolean;
  citationNumbers: readonly number[];
  selected?: number;
  onCite: (n: number) => void;
};

const base: Components = {
  p: ({ children }) => <p className="mb-3 max-w-[68ch] last:mb-0">{children}</p>,
  strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
  h3: ({ children }) => <h3 className="mt-5 mb-2 text-base font-bold tracking-[-0.005em]">{children}</h3>,
  ul: ({ children }) => <ul className="mb-3 ml-5 list-disc space-y-1">{children}</ul>,
  ol: ({ children }) => <ol className="mb-3 ml-5 list-decimal space-y-1">{children}</ol>,
  a: ({ children, href }) => (
    <a href={href} target="_blank" rel="noreferrer" className="text-mcp underline">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="border-rule bg-paper my-2 mb-3.5 overflow-x-auto rounded-[10px] border">
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  ),
  th: ({ children }) => (
    <th className="border-rule bg-paper-2 text-ink-2 border-b px-3.5 py-2.25 text-left text-[12.5px] font-semibold">
      {children}
    </th>
  ),
  td: ({ children }) => (
    <td className="border-rule-2 border-b px-3.5 py-2.5 align-top first:w-[30%] first:font-semibold [tr:last-child_&]:border-b-0">
      {children}
    </td>
  ),
};

/** The answer as Markdown, its `[n]` markers drawn as roundels that open the cited block. */
export const Answer = memo(function Answer({ text, streaming, citationNumbers, selected, onCite }: Props) {
  const t = useTranslations("chat");
  const known = useMemo(() => new Set(citationNumbers), [citationNumbers]);
  const components = useMemo(
    () =>
      ({
        ...base,
        [CITE_TAG]: ({ n }: { n: string }) => (
          <CitationRoundel
            n={Number(n)}
            label={t("citation", { n })}
            selected={selected === Number(n)}
            onSelect={onCite}
          />
        ),
      }) as Components,
    [onCite, selected, t],
  );
  return (
    <div className="text-[15.5px] leading-[1.65]">
      <ReactMarkdown remarkPlugins={[remarkGfm, [remarkCitations, { known }]]} components={components}>
        {text}
      </ReactMarkdown>
      {streaming && <Caret />}
    </div>
  );
});
