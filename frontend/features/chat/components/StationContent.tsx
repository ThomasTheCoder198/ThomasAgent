"use client";

import { Check, ExternalLink, FileText } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

import { KB_TOOLS } from "../contract";
import type { ReasoningStation, ToolStation } from "../run-view";

const FILE_TAIL = 18;

const text = (value: unknown) => (typeof value === "string" ? value : undefined);

/** Long file names keep their distinguishing tail: "…hoan-tien-2026.pdf". */
function tail(fileName: string) {
  return fileName.length > FILE_TAIL ? `…${fileName.slice(-FILE_TAIL)}` : fileName;
}

export function Caret() {
  return (
    <span
      aria-hidden
      className="bg-foreground animate-caret ml-0.5 inline-block h-[1.05em] w-0.5 align-[-0.1875rem]"
    />
  );
}

export function ReasoningBody({ station }: { station: ReasoningStation }) {
  return (
    <p
      className={cn(
        "mt-1 max-w-prose text-sm leading-relaxed",
        station.live ? "text-foreground" : "text-muted-foreground",
      )}
    >
      {station.text}
      {station.live && <Caret />}
    </p>
  );
}

function KbSearchBody({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  // Until Task 2c-3 types the real ragx payloads, the output is read defensively.
  const input = (station.input ?? {}) as Record<string, unknown>;
  const output = station.output as Record<string, unknown> | undefined;
  const query = text(input.query);
  const chunks = typeof output?.chunks === "number" ? output.chunks : 0;
  const documents = Array.isArray(output?.documents)
    ? output.documents.filter((doc): doc is string => typeof doc === "string")
    : [];
  return (
    <div className="mt-1 text-sm">
      {query && (
        <span className="bg-kb-tint text-foreground inline-block rounded-sm px-2 py-1 text-[0.8125rem]">
          “{query}”
        </span>
      )}
      {output && (
        <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-2 text-[0.8125rem]">
          {t("chunks", { chunks, docs: documents.length })}
          {documents.map((doc) => (
            <span
              key={doc}
              title={doc}
              className="border-border bg-card inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-xs"
            >
              <FileText aria-hidden className="size-3" />
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
    <p className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      {ids.map((id) => (
        <span key={id} className="tabular text-foreground text-xs">
          {id}
        </span>
      ))}
      {station.ownerApproved && (
        <span className="inline-flex items-center gap-1">
          <Check aria-hidden className="text-success size-3.5" />
          {t("approvedNote")}
        </span>
      )}
      {link && (
        <a
          href={link}
          target="_blank"
          rel="noreferrer"
          className="text-primary inline-flex items-center gap-1 hover:underline"
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
  if (station.state === "error") return <p className="text-destructive mt-1 text-sm">{station.errorText}</p>;
  if (station.state === "denied")
    return <p className="text-muted-foreground mt-1 text-sm">{t("toolDenied")}</p>;
  if (station.name === KB_TOOLS.search) return <KbSearchBody station={station} />;
  if (station.name === KB_TOOLS.readSection) return null;
  return <ToolOutcome station={station} />;
}

export function ToolLabel({ station }: { station: ToolStation }) {
  const t = useTranslations("chat");
  const b = (chunks: ReactNode) => <b className="font-semibold">{chunks}</b>;
  if (station.name === KB_TOOLS.search) return t.rich("toolKbSearch", { source: station.source, b });
  if (station.name === KB_TOOLS.readSection) {
    const input = (station.input ?? {}) as Record<string, unknown>;
    return t.rich("toolKbRead", { section: text(input.section) ?? "", file: text(input.fileName) ?? "", b });
  }
  return t.rich("toolGeneric", { source: station.source, title: station.title, b });
}
