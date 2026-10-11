"use client";

import { useLocale } from "next-intl";
import { useCallback, useEffect, useState } from "react";

import type { Locale } from "@/i18n/config";
import type { KbFile } from "@/lib/ragx/types";

import { listFiles } from "./api";
import { FILE_POLL_INTERVAL_MS, FINAL_FILE_STATUSES } from "./contract";

/** Rows of a fresh first page replace the rows with the same id; rows it adds go on top; the rest stay. */
export function mergeFirstPage(current: readonly KbFile[], fresh: readonly KbFile[]): KbFile[] {
  const freshById = new Map(fresh.map((file) => [file.id, file]));
  const known = new Set(current.map((file) => file.id));
  return [
    ...fresh.filter((file) => !known.has(file.id)),
    ...current.map((file) => freshById.get(file.id) ?? file),
  ];
}

const withoutDuplicates = (files: readonly KbFile[], more: readonly KbFile[]) => [
  ...files,
  ...more.filter((file) => !files.some((f) => f.id === file.id)),
];

/** While any file is still being ingested, and the tab is visible, the first page is re-read to follow it. */
function usePolling(orgId: string, kbId: string, active: boolean, apply: (fresh: KbFile[]) => void) {
  const locale = useLocale() as Locale;
  const [pollError, setPollError] = useState<unknown>();
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => {
      if (document.visibilityState !== "visible") return;
      listFiles(orgId, kbId, null, locale).then(
        ({ data }) => {
          apply(data);
          setPollError(undefined);
        },
        (error: unknown) => setPollError(error),
      );
    }, FILE_POLL_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [active, apply, kbId, locale, orgId]);
  return pollError;
}

export function useFiles(orgId: string, kbId: string, initialFiles: KbFile[], initialCursor: string | null) {
  const locale = useLocale() as Locale;
  const [files, setFiles] = useState(initialFiles);
  const [cursor, setCursor] = useState(initialCursor);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<unknown>();

  const apply = useCallback((fresh: KbFile[]) => setFiles((all) => mergeFirstPage(all, fresh)), []);
  const upsert = useCallback(
    (file: KbFile) => setFiles((all) => [file, ...all.filter((f) => f.id !== file.id)]),
    [],
  );
  const remove = useCallback((id: string) => setFiles((all) => all.filter((f) => f.id !== id)), []);
  const pollError = usePolling(
    orgId,
    kbId,
    files.some((file) => !FINAL_FILE_STATUSES.has(file.status)),
    apply,
  );

  async function loadMore() {
    setLoadingMore(true);
    setMoreError(undefined);
    try {
      const { data, page } = await listFiles(orgId, kbId, cursor, locale);
      setFiles((all) => withoutDuplicates(all, data));
      setCursor(page.cursor);
    } catch (error) {
      setMoreError(error);
    } finally {
      setLoadingMore(false);
    }
  }

  return { files, cursor, loadingMore, loadMore, upsert, remove, pollError, moreError };
}
