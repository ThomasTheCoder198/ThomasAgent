"use client";

import { LibraryBig, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback } from "react";

import { IconButton } from "@/components/ui/Button";
import { tabPanelId, Tabs, type TabItem } from "@/components/ui/Tabs";
import { useDismiss } from "@/components/ui/useDismiss";
import { cn } from "@/lib/cn";

import { LineBadge } from "@/components/metro/LineBadge";
import type { RunUsage } from "../contract";
import type { Citation, Station } from "../run-view";
import { DocumentPageView } from "./DocumentPageView";
import { RunInspector } from "./RunInspector";
import { SourceList } from "./SourceList";
import { useDocumentPage } from "./useDocumentPage";

export type EvidenceTab = "evidence" | "sources" | "inspector";

/** Sources listed under the page preview; the Sources tab shows them all. */
const FOOTER_SOURCES = 3;

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

function SignHeader({ citation, onClose }: { citation: Citation | undefined; onClose: () => void }) {
  const t = useTranslations("evidence");
  const locator = citation?.locator;
  return (
    <header className="bg-sign text-sign-ink px-4.5 pt-3.5 pb-3">
      {/* The file is the sign's heading; the KB name plate rides in the meta row with page and section. */}
      <div className="flex items-center gap-2">
        <h2 className="min-w-0 flex-1 truncate text-[15px] font-semibold tracking-[-0.005em]">
          {citation?.fileName ?? t("region")}
        </h2>
        <IconButton tone="sign" label={t("close")} onClick={onClose} className="size-7">
          <X aria-hidden className="size-4" />
        </IconButton>
      </div>
      {locator && (
        <p className="text-sign-ink-2 mt-1.5 flex min-w-0 items-center gap-2 text-xs">
          <LineBadge line="kb" icon={LibraryBig}>
            {locator.kbName}
          </LineBadge>
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
  if (!citation?.locator) return <p className="text-ink-2 p-5 text-sm">{t("pickCitation")}</p>;
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
  if (state.status === "loading")
    return <div className="bg-page/60 m-4.5 h-80 animate-pulse rounded" aria-busy />;
  if (state.status === "error")
    return (
      <p className="text-app p-5 text-sm" role="alert">
        {state.message || t("loadFailed")}
      </p>
    );
  return (
    <div className="p-4.5 pl-10">
      <DocumentPageView page={state.page} {...rest} />
    </div>
  );
}

const TABS_ID = "evidence";

function EvidenceTabs({
  tab,
  onTab,
  sourceCount,
}: {
  tab: EvidenceTab;
  onTab: (tab: EvidenceTab) => void;
  sourceCount: number;
}) {
  const t = useTranslations("evidence");
  const tabs: Array<TabItem<EvidenceTab>> = [
    { id: "evidence", label: t("tabEvidence") },
    {
      id: "sources",
      label: (
        <>
          {t("tabSources")} <span className="tabular text-[11px]">{sourceCount}</span>
        </>
      ),
    },
    { id: "inspector", label: t("tabInspector") },
  ];
  return (
    <Tabs
      items={tabs}
      value={tab}
      onChange={onTab}
      idPrefix={TABS_ID}
      label={t("region")}
      className="border-rule bg-paper border-b px-3"
    />
  );
}

/**
 * The right-hand platform sign. Wide screens keep it as a third column; medium screens slide it over the
 * thread; phones get a bottom sheet (spec §10.1).
 */
export function EvidencePanel(props: Props) {
  const { open, tab, onTab, onClose, citations, selected, onSelect, stations, usage, runId } = props;
  const t = useTranslations("evidence");
  const citation = citations.find((c) => c.n === selected);
  const close = useCallback(() => onClose(), [onClose]);
  useDismiss(open, close);
  return (
    <>
      {open && <div className="bg-scrim fixed inset-0 z-30 xl:hidden" aria-hidden onClick={onClose} />}
      <aside
        aria-label={t("region")}
        className={cn(
          "bg-paper z-40 flex min-h-0 flex-col overflow-hidden",
          "max-md:shadow-sheet max-md:border-rule fixed inset-x-0 bottom-0 h-[78dvh] rounded-t-[22px] max-md:border-t",
          "md:border-rule md:shadow-lift md:inset-y-0 md:right-0 md:left-auto md:h-auto md:w-[clamp(22rem,30vw,30rem)] md:rounded-none md:border-l",
          "xl:static xl:shadow-none",
          "ease-out-expo transition-transform duration-300",
          open ? "translate-0" : "max-md:translate-y-full md:translate-x-full xl:hidden",
        )}
      >
        <span
          aria-hidden
          className="bg-rule-strong mx-auto mt-2 mb-1 h-1.25 w-10 shrink-0 rounded-full md:hidden"
        />
        <SignHeader citation={citation} onClose={onClose} />
        <EvidenceTabs tab={tab} onTab={onTab} sourceCount={citations.length} />
        <div
          role="tabpanel"
          id={tabPanelId(TABS_ID, tab)}
          aria-labelledby={`${TABS_ID}-tab-${tab}`}
          className={cn("min-h-0 flex-1 overflow-y-auto", tab === "evidence" && "bg-pageview")}
        >
          {tab === "evidence" && (
            <EvidenceBody citation={citation} citations={citations} selected={selected} onSelect={onSelect} />
          )}
          {tab === "sources" && <SourceList citations={citations} selected={selected} onSelect={onSelect} />}
          {tab === "inspector" && <RunInspector stations={stations} usage={usage} runId={runId} />}
        </div>
        {tab === "evidence" && citations.length > 0 && (
          <div className="border-rule hidden border-t md:block">
            <SourceList
              citations={citations.slice(0, FOOTER_SOURCES)}
              selected={selected}
              onSelect={onSelect}
              compact
            />
          </div>
        )}
      </aside>
    </>
  );
}
