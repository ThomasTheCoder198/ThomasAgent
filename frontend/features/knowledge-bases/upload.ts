import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";
import type { DownloadUrl, KbFile, UploadTicket } from "@/lib/ragx/types";

import { RAGX_PATHS } from "./contract";

const FILE_FIELD = "file";
const S3_CODE = /<Code>([^<]+)<\/Code>/;
export const NETWORK_CODE = "NETWORK";
const SUCCESS_MIN = 200;
const SUCCESS_MAX = 299;

/** The storage leg failed: MinIO answers XML with an S3 code and gives no request id. */
export class StorageUploadError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`storage ${status} ${code}`);
    this.name = "StorageUploadError";
  }
}

export function presignUpload(orgId: string, kbId: string, file: File, locale: Locale, signal: AbortSignal) {
  return apiFetch<UploadTicket>(RAGX_PATHS.uploads(orgId, kbId), {
    method: "POST",
    body: JSON.stringify({ name: file.name, sizeBytes: file.size }),
    locale,
    signal,
  });
}

/** XHR, not fetch: only XHR reports upload progress. Straight to storage, so no cookie, CSRF or request id. */
export function postToStorage(
  ticket: UploadTicket,
  file: File,
  onProgress: (loaded: number, total: number) => void,
  signal: AbortSignal,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", ticket.upload.url);
    xhr.withCredentials = false;
    xhr.upload.onprogress = (e) => onProgress(e.loaded, e.lengthComputable ? e.total : file.size);
    xhr.onload = () =>
      xhr.status >= SUCCESS_MIN && xhr.status <= SUCCESS_MAX
        ? resolve()
        : reject(
            new StorageUploadError(xhr.status, S3_CODE.exec(xhr.responseText)?.[1] ?? String(xhr.status)),
          );
    xhr.onerror = () => reject(new StorageUploadError(0, NETWORK_CODE));
    signal.addEventListener(
      "abort",
      () => {
        xhr.abort();
        reject(new DOMException("Upload cancelled", "AbortError"));
      },
      { once: true },
    );
    const form = new FormData();
    // S3 POST ignores fields after the file: the policy fields go first.
    for (const [name, value] of Object.entries(ticket.upload.fields)) form.append(name, value);
    form.append(FILE_FIELD, file, file.name);
    xhr.send(form);
  });
}

/** Idempotent by file id, so a retry or a double click cannot create a second copy (P10). */
export function confirmUpload(
  orgId: string,
  kbId: string,
  fileId: string,
  locale: Locale,
  signal: AbortSignal,
) {
  return apiFetch<KbFile>(RAGX_PATHS.confirm(orgId, kbId, fileId), { method: "POST", locale, signal });
}

export function fetchDownloadUrl(orgId: string, kbId: string, fileId: string, locale: Locale) {
  return apiFetch<DownloadUrl>(RAGX_PATHS.downloadUrl(orgId, kbId, fileId), { locale });
}
