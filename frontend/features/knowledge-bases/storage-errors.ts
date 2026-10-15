import { INTERRUPTED_CODE, NETWORK_CODE, type StorageUploadError } from "./upload";

/** What a failed browser → storage POST means to the operator; each has a `kb.storage.*` message (P-F17). */
export const STORAGE_PROBLEMS = [
  "sizeMismatch",
  "denied",
  "timeout",
  "busy",
  "fault",
  "unreachable",
  "interrupted",
  "rejected",
] as const;
export type StorageProblem = (typeof STORAGE_PROBLEMS)[number];

const HTTP_INTERNAL_ERROR = 500;
const HTTP_SERVICE_UNAVAILABLE = 503;

/**
 * S3 codes MinIO answers a presigned POST with. AccessDenied covers an expired policy; clock skew and a bad
 * signature also mean the ticket is unusable, and Retry presigns again after any 403.
 */
const BY_CODE: Readonly<Record<string, StorageProblem>> = {
  // The policy pins the exact size, so a larger or smaller body means the file changed after it was picked.
  EntityTooLarge: "sizeMismatch",
  EntityTooSmall: "sizeMismatch",
  AccessDenied: "denied",
  RequestTimeTooSkewed: "denied",
  SignatureDoesNotMatch: "denied",
  InvalidAccessKeyId: "denied",
  RequestTimeout: "timeout",
  SlowDown: "busy",
  ServiceUnavailable: "busy",
  InternalError: "fault",
  [NETWORK_CODE]: "unreachable",
  [INTERRUPTED_CODE]: "interrupted",
};

export function storageProblem(error: StorageUploadError): StorageProblem {
  const known = BY_CODE[error.code];
  if (known) return known;
  if (error.status === HTTP_SERVICE_UNAVAILABLE) return "busy";
  if (error.status >= HTTP_INTERNAL_ERROR) return "fault";
  return "rejected";
}
