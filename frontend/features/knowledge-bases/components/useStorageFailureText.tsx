"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { storageProblem } from "../storage-errors";
import type { StorageUploadError } from "../upload";

/** The localized line for a storage failure, with the S3 code in mono; storage gives no request id (P-F17). */
export function useStorageFailureText(): (error: StorageUploadError) => ReactNode {
  const t = useTranslations("kb");
  return (error) =>
    t.rich(`storage.${storageProblem(error)}`, {
      code: error.code,
      mono: (chunks) => <span className="tabular">{chunks}</span>,
    });
}
