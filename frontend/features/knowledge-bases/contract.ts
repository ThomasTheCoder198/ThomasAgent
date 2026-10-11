/** Ragx's KB API, reached only through core's generic app proxy (spec ragx-custom-app §14.2). */
import type { FileStatus } from "@/lib/ragx/types";

export const fileStatuses = [
  "pending",
  "parsing",
  "enriching",
  "chunking",
  "embedding",
  "indexing",
  "ready",
  "failed",
  "degraded",
] as const satisfies readonly FileStatus[];
/** A file in one of these states will not change without the operator (retry, delete). */
export const FINAL_FILE_STATUSES: ReadonlySet<FileStatus> = new Set(["ready", "failed", "degraded"]);

const enc = encodeURIComponent;
const app = (orgId: string) => `/api/v1/orgs/${enc(orgId)}/apps/ragx`;

export const RAGX_PATHS = {
  knowledgeBases: (orgId: string) => `${app(orgId)}/knowledge-bases`,
  knowledgeBase: (orgId: string, kbId: string) => `${app(orgId)}/knowledge-bases/${enc(kbId)}`,
  files: (orgId: string, kbId: string) => `${RAGX_PATHS.knowledgeBase(orgId, kbId)}/files`,
  file: (orgId: string, kbId: string, fileId: string) => `${RAGX_PATHS.files(orgId, kbId)}/${enc(fileId)}`,
  retry: (orgId: string, kbId: string, fileId: string) => `${RAGX_PATHS.file(orgId, kbId, fileId)}/retry`,
  uploads: (orgId: string, kbId: string) => `${RAGX_PATHS.knowledgeBase(orgId, kbId)}/uploads`,
  confirm: (orgId: string, kbId: string, fileId: string) => `${RAGX_PATHS.file(orgId, kbId, fileId)}/confirm`,
  downloadUrl: (orgId: string, kbId: string, fileId: string) =>
    `${RAGX_PATHS.file(orgId, kbId, fileId)}/download-url`,
} as const;

export const kbPage = (orgId: string, kbId: string) => `/orgs/${enc(orgId)}/knowledge-bases/${enc(kbId)}`;
export const KB_NAME_MAX_LENGTH = 200;
export const FILE_PAGE_LIMIT = 20;
/** Ingest steps take seconds; this keeps a busy list current without hammering core. */
export const FILE_POLL_INTERVAL_MS = 4000;
/**
 * Provisioning is synchronous, so a `pending` org means a core process died mid-call. The station refreshes
 * itself for about a minute (15 x 4 s) in case a late answer lands, then stops and leaves Retry (frontend rule 8).
 */
export const ORG_PENDING_MAX_REFRESHES = 15;
