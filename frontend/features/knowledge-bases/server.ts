import "server-only";

import { ApiError } from "@/lib/api/client";
import { serverFetch, serverFetchPage } from "@/lib/api/server";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KbFile, KnowledgeBase } from "@/lib/ragx/types";

import { FILE_PAGE_LIMIT, RAGX_PATHS } from "./contract";

/** A page can tell "not there" (404 page) from "ragx or core is down" (retryable error with the request id). */
export type KbLoad<T> =
  | { ok: true; data: T }
  | {
      ok: false;
      notFound: boolean;
      /** Decided by status, never by the code RAGX_UNAVAILABLE: core shows 5xx as INTERNAL_ERROR (F17). */
      serviceDown: boolean;
      code: string;
      message: string;
      requestId?: string;
    };

const NOT_FOUND_CODES = new Set<string>([ErrorCode.NOT_FOUND, ErrorCode.KB_NOT_FOUND]);
const SERVER_ERROR_MIN = 500;

async function load<T>(fetcher: () => Promise<T>): Promise<KbLoad<T>> {
  try {
    return { ok: true, data: await fetcher() };
  } catch (err) {
    if (!(err instanceof ApiError)) throw err;
    return {
      ok: false,
      notFound: NOT_FOUND_CODES.has(err.code),
      serviceDown: err.status >= SERVER_ERROR_MIN,
      code: err.code,
      message: err.message,
      requestId: err.requestId,
    };
  }
}

export function listKnowledgeBases(orgId: string) {
  return load(() => serverFetch<KnowledgeBase[]>(RAGX_PATHS.knowledgeBases(orgId)));
}

export function getKnowledgeBasePage(orgId: string, kbId: string) {
  return load(async () => {
    const [knowledgeBase, files] = await Promise.all([
      serverFetch<KnowledgeBase>(RAGX_PATHS.knowledgeBase(orgId, kbId)),
      serverFetchPage<KbFile[]>(`${RAGX_PATHS.files(orgId, kbId)}?limit=${FILE_PAGE_LIMIT}`),
    ]);
    return { knowledgeBase, files: files.data, cursor: files.page.cursor };
  });
}
