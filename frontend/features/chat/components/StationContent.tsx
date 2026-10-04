"use client";

import { Check, ExternalLink, FileText } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { KB_TOOLS, type KbReadInput, type KbSearchInput, type KbSearchOutput } from "../contract";
import type { ReasoningStation, ToolStation } from "../run-view";

const FILE_TAIL = 18;

/** Long file names keep their distinguishing tail: "…hoan-tien-2026.pdf". */
function tail(fileName: string) {
  return fileName.length > FILE_TAIL ? `…${fileName.slice(-FILE_TAIL)}` : fileName;
}

export function Caret() {
  return (
    <span aria-hidden className="bg-ink animate-caret ml-0.5 inline-block h-[1.05em] w-0.5 align-[-3px]" />
  );
}

export function ReasoningBody({ station }: { station: ReasoningStation }) {
  return (
    <p
      className={cn(
        "mt-1.5 max-w-[62ch] text-[13.5px] leading-relaxed",
        station.live ? "text-ink" : "text-ink-2",
      )}
    >
      {station.text}
      {station.live && <Caret />}
    </p>
  );
}

function KbSearchBody({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  const input = station.input as Partial<KbSearchInput>;
  const output = station.output as KbSearchOutput | undefined;
  return (
    <div className="mt-1.5 text-[13.5px]">
      {input.query && (
        <span className="bg-kb-tint text-ink inline-block rounded-md px-2.5 py-0.5 text-[13px]">
          “{input.query}”
        </span>
      )}
      {output && (
        <div className="text-ink-2 mt-2 flex flex-wrap items-center gap-1.5 text-[13px]">
          {t("chunks", { chunks: output.chunks, docs: output.documents.length })}
          {output.documents.map((doc) => (
            <span
              key={doc}
              title={doc}
              className="border-rule bg-paper inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[12.5px]"
            >
              <FileText aria-hidden className="size-3.25" />
              {tail(doc)}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

const SUMMARY_FIELDS = 2;
const LINK_FIELD = "url";

/** What the tool produced, as identifiers plus a way out to the system it touched. */
function ToolOutcome({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  const output = (station.output ?? {}) as Record<string, unknown>;
  const ids = Object.entries(output)
    .filter(([key, value]) => key !== LINK_FIELD && (typeof value === "string" || typeof value === "number"))
    .slice(0, SUMMARY_FIELDS)
    .map(([, value]) => String(value));
  const link = typeof output[LINK_FIELD] === "string" ? output[LINK_FIELD] : undefined;
  if (ids.length === 0 && !link && !station.ownerApproved) return null;
  return (
    <p className="text-ink-2 mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[13.5px]">
      {ids.map((id) => (
        <span key={id} className="tabular text-ink text-[12.5px] font-medium">
          {id}
        </span>
      ))}
      {station.ownerApproved && (
        <span className="inline-flex items-center gap-1">
          <Check aria-hidden className="text-kb size-3.5" />
          {t("approvedNote")}
        </span>
      )}
      {link && (
        <a
          href={link}
          target="_blank"
          rel="noreferrer"
          className="text-mcp inline-flex items-center gap-1 hover:underline"
        >
          {t("openLink", { source: station.source })}
          <ExternalLink aria-hidden className="size-3.5" />
        </a>
      )}
    </p>
  );
}

export function ToolBody({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  if (station.state === "error") return <p className="text-app mt-1.5 text-[13.5px]">{station.errorText}</p>;
  if (station.state === "denied") return <p className="text-ink-3 mt-1.5 text-[13.5px]">{t("toolDenied")}</p>;
  if (station.name === KB_TOOLS.search) return <KbSearchBody station={station} />;
  if (station.name === KB_TOOLS.readSection) return null;
  return <ToolOutcome station={station} />;
}

export function ToolLabel({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  const b = (chunks: ReactNode) => <b className="font-semibold">{chunks}</b>;
  if (station.name === KB_TOOLS.search) return t.rich("toolKbSearch", { source: station.source, b });
  if (station.name === KB_TOOLS.readSection) {
    const input = station.input as Partial<KbReadInput>;
    return t.rich("toolKbRead", { section: input.section ?? "", file: input.fileName ?? "", b });
  }
  return t.rich("toolGeneric", { source: station.source, title: station.title, b });
}
