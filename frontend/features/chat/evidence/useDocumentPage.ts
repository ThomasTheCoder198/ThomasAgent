"use client";

import { useLocale } from "next-intl";
import { useEffect, useState } from "react";

import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";

import { CHAT_PATHS, type DocumentPage } from "../contract";

export type PageState =
  { status: "loading" } | { status: "ready"; page: DocumentPage } | { status: "error"; message: string };

/**
 * Loads one source page. Callers key the consuming component by document and page, so a new citation
 * starts from a fresh "loading" state instead of flashing the previous page.
 */
export function useDocumentPage(documentId: string, page: number): PageState {
  const locale = useLocale() as Locale;
  const [state, setState] = useState<PageState>({ status: "loading" });
  useEffect(() => {
    const controller = new AbortController();
    apiFetch<DocumentPage>(CHAT_PATHS.documentPage(documentId, page), { locale, signal: controller.signal })
      .then((data) => setState({ status: "ready", page: data }))
      .catch((err: unknown) => {
        if (!controller.signal.aborted)
          setState({ status: "error", message: err instanceof Error ? err.message : "" });
      });
    return () => controller.abort();
  }, [documentId, page, locale]);
  return state;
}
