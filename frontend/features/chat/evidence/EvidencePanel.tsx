"use client";

import { LibraryBig, X } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ElementType } from "react";

import { Button } from "@/components/ui/Button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/Sheet";
import { Skeleton } from "@/components/ui/Skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { useIsBelowMd, useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/lib/cn";

import { SourceBadge } from "../components/SourceBadge";
import type { RunUsage } from "../contract";
import type { Citation, Station } from "../run-view";
import { DocumentPageView } from "./DocumentPageView";
import { RunInspector } from "./RunInspector";
import { SourceList } from "./SourceList";
import { useDocumentPage } from "./useDocumentPage";

export type EvidenceTab = "evidence" | "sources" | "inspector";

/** Sources listed under the page preview; the Sources tab shows them all. */
const FOOTER_SOURCES = 3;

/** Where Tailwind's `xl` starts (16px root font): from here the panel is a column, below it a sheet. */
const XL_MIN_WIDTH_PX = 1280;
const DOCKED_QUERY = `(min-width: ${XL_MIN_WIDTH_PX}px)`;

type Props = {
  open: boolean;
  tab: EvidenceTab;
  onTab: (tab: EvidenceTab) => void;
  onClose: () => void;
  citations: Citation[];
  selected: number | undefined;
  onSelect: (n: number) => void;
  stations: Station[];
  usage: RunUsage | undefined;
  runId: string | undefined;
};

/** The file is the panel's heading; the KB name plate rides in the meta row with page and section. */
function PanelHeader({
  citation,
  onClose,
  Heading,
}: {
  citation: Citation | undefined;
  onClose: () => void;
  Heading: ElementType;
}) {
  const t = useTranslations("evidence");
  const locator = citation?.locator;
  return (
    <header className="border-border border-b px-4 py-3">
      <div className="flex items-center gap-2">
        <Heading className="min-w-0 flex-1 truncate text-base font-semibold">
          {citation?.fileName ?? t("region")}
        </Heading>
        <Button variant="ghost" size="icon-sm" aria-label={t("close")} title={t("close")} onClick={onClose}>
          <X aria-hidden />
        </Button>
      </div>
      {locator && (
        <p className="text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-xs">
          <SourceBadge line="kb" icon={LibraryBig}>
            {locator.kbName}
          </SourceBadge>
          <span className="tabular truncate">
            {t("page", { page: locator.page, total: locator.pageCount })}
            {locator.section && ` · ${locator.section}`}
          </span>
        </p>
      )}
    </header>
  );
}

function EvidenceBody({
  citation,
  citations,
  selected,
  onSelect,
}: { citation: Citation | undefined } & Pick<Props, "citations" | "selected" | "onSelect">) {
  const t = useTranslations("evidence");
  if (!citation?.locator) return <p className="text-muted-foreground p-4 text-sm">{t("pickCitation")}</p>;
  const { documentId, page } = citation.locator;
  return (
    <LoadedPage
      key={`${documentId}:${page}`}
      documentId={documentId}
      page={page}
      citations={citations}
      selected={selected}
      onSelect={onSelect}
    />
  );
}

function LoadedPage({
  documentId,
  page,
  ...rest
}: { documentId: string; page: number } & Pick<Props, "citations" | "selected" | "onSelect">) {
  const t = useTranslations("evidence");
  const state = useDocumentPage(documentId, page);
  if (state.status === "loading") return <Skeleton aria-busy className="bg-page/60 m-4 ml-10 h-80" />;
  if (state.status === "error")
    return (
      <p className="text-destructive p-4 text-sm" role="alert">
        {state.message || t("loadFailed")}
      </p>
    );
  return (
    <div className="p-4 pl-10">
      <DocumentPageView page={state.page} {...rest} />
    </div>
  );
}

function EvidenceTabList({ sourceCount }: { sourceCount: number }) {
  const t = useTranslations("evidence");
  return (
    <TabsList aria-label={t("region")} className="px-3">
      <TabsTrigger value="evidence">{t("tabEvidence")}</TabsTrigger>
      <TabsTrigger value="sources">
        {t("tabSources")} <span className="tabular text-xs">{sourceCount}</span>
      </TabsTrigger>
      <TabsTrigger value="inspector">{t("tabInspector")}</TabsTrigger>
    </TabsList>
  );
}

const isEvidenceTab = (value: string): value is EvidenceTab =>
  value === "evidence" || value === "sources" || value === "inspector";

function PanelTabs({ tab, onTab, citations, selected, onSelect, stations, usage, runId }: Props) {
  const citation = citations.find((c) => c.n === selected);
  return (
    <Tabs
      value={tab}
      onValueChange={(next) => isEvidenceTab(next) && onTab(next)}
      className="min-h-0 flex-1 gap-0"
    >
      <EvidenceTabList sourceCount={citations.length} />
      {/* The cited document stays light paper in both themes: it stands for the source file, not the UI. */}
      <TabsContent value="evidence" className="bg-pageview min-h-0 flex-1 overflow-y-auto">
        <EvidenceBody citation={citation} citations={citations} selected={selected} onSelect={onSelect} />
      </TabsContent>
      <TabsContent value="sources" className="min-h-0 flex-1 overflow-y-auto">
        <SourceList citations={citations} selected={selected} onSelect={onSelect} />
      </TabsContent>
      <TabsContent value="inspector" className="min-h-0 flex-1 overflow-y-auto">
        <RunInspector stations={stations} usage={usage} runId={runId} />
      </TabsContent>
      {tab === "evidence" && citations.length > 0 && (
        <div className="border-border hidden border-t md:block">
          <SourceList
            citations={citations.slice(0, FOOTER_SOURCES)}
            selected={selected}
            onSelect={onSelect}
            compact
          />
        </div>
      )}
    </Tabs>
  );
}

/**
 * Where the evidence lives follows the room: a docked column from xl, a right sheet from md, a bottom sheet
 * (85dvh) on phones (spec §10.1). Sheets are modal, so they bring focus handling and Escape; the docked
 * column is part of the page and closes with its button.
 */
export function EvidencePanel(props: Props) {
  const { open, onClose, citations, selected } = props;
  const t = useTranslations("evidence");
  const docked = useMediaQuery(DOCKED_QUERY);
  const phone = useIsBelowMd();
  const citation = citations.find((c) => c.n === selected);

  if (docked) {
    if (!open) return null;
    return (
      <aside
        aria-label={t("region")}
        className="bg-card border-border animate-in slide-in-from-right-8 flex min-h-0 w-[clamp(22rem,30vw,30rem)] shrink-0 flex-col border-l duration-300"
      >
        <PanelHeader citation={citation} onClose={onClose} Heading="h2" />
        <PanelTabs {...props} />
      </aside>
    );
  }
  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        showCloseButton={false}
        aria-describedby={undefined}
        className={cn("min-h-0 gap-0 p-0", phone ? "h-[85dvh]" : "w-[clamp(22rem,30vw,30rem)] max-w-[85vw]")}
      >
        <SheetTitle className="sr-only">{t("region")}</SheetTitle>
        <SheetDescription className="sr-only">{t("pickCitation")}</SheetDescription>
        {phone && <span aria-hidden className="bg-input mx-auto mt-2 h-1 w-10 shrink-0 rounded-full" />}
        <PanelHeader citation={citation} onClose={onClose} Heading="h2" />
        <PanelTabs {...props} />
      </SheetContent>
    </Sheet>
  );
}
