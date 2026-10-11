import type { Locale } from "@/i18n/config";
import { apiFetch, apiFetchPage, type Page } from "@/lib/api/client";
import type { KbFile, KnowledgeBase, KnowledgeBaseWrite } from "@/lib/ragx/types";

import { FILE_PAGE_LIMIT, RAGX_PATHS } from "./contract";

const json = (body: unknown) => JSON.stringify(body);

export function createKnowledgeBase(orgId: string, input: KnowledgeBaseWrite, locale: Locale) {
  return apiFetch<KnowledgeBase>(RAGX_PATHS.knowledgeBases(orgId), {
    method: "POST",
    body: json(input),
    locale,
  });
}

export function renameKnowledgeBase(orgId: string, kbId: string, input: KnowledgeBaseWrite, locale: Locale) {
  return apiFetch<KnowledgeBase>(RAGX_PATHS.knowledgeBase(orgId, kbId), {
    method: "PATCH",
    body: json(input),
    locale,
  });
}

export function deleteKnowledgeBase(orgId: string, kbId: string, locale: Locale) {
  return apiFetch<undefined>(RAGX_PATHS.knowledgeBase(orgId, kbId), { method: "DELETE", locale });
}

/** `cursor` null reads the first page; the answer carries the next cursor. */
export function listFiles(
  orgId: string,
  kbId: string,
  cursor: string | null,
  locale: Locale,
): Promise<{ data: KbFile[]; page: Page }> {
  const query = new URLSearchParams({ limit: String(FILE_PAGE_LIMIT) });
  if (cursor) query.set("cursor", cursor);
  return apiFetchPage<KbFile[]>(`${RAGX_PATHS.files(orgId, kbId)}?${query}`, { locale });
}

export function retryFile(orgId: string, kbId: string, fileId: string, locale: Locale) {
  return apiFetch<KbFile>(RAGX_PATHS.retry(orgId, kbId, fileId), { method: "POST", locale });
}

export function deleteFile(orgId: string, kbId: string, fileId: string, locale: Locale) {
  return apiFetch<undefined>(RAGX_PATHS.file(orgId, kbId, fileId), { method: "DELETE", locale });
}
