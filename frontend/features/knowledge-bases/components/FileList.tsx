"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { useRef } from "react";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { EmptyState } from "@/features/shell/EmptyState";
import { describeApiError } from "@/lib/api/client";
import type { KbFile } from "@/lib/ragx/types";
import { AUTO_DISMISS_MS } from "@/lib/notify";
import { useNotify } from "@/lib/useNotify";

import { PAGE_COLUMN } from "../layout";
import { StorageUploadError } from "../upload";
import { useFiles } from "../useFiles";
import { useUploadQueue } from "../useUploadQueue";
import { FileRow } from "./FileRow";
import { UploadDropZone } from "./UploadDropZone";
import { UploadRow } from "./UploadRow";
import { useStorageFailureText } from "./useStorageFailureText";

/** One toast per batch: every finished upload updates it, and a quiet spell starts a new batch. */
const UPLOADS_TOAST_ID = "kb-uploads";

type Props = { orgId: string; kbId: string; initialFiles: KbFile[]; initialCursor: string | null };

/** The drop zone, upload rows first (files leaving), then the documents; the list follows ingest while anything is in flight. */
export function FileList({ orgId, kbId, initialFiles, initialCursor }: Props) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const router = useRouter();
  const notify = useNotify();
  const storageText = useStorageFailureText();
  const list = useFiles(orgId, kbId, initialFiles, initialCursor);
  const batch = useRef({ count: 0, timer: undefined as ReturnType<typeof setTimeout> | undefined });
  const queue = useUploadQueue(
    orgId,
    kbId,
    (file) => {
      list.upsert(file);
      const current = batch.current;
      current.count += 1;
      clearTimeout(current.timer);
      current.timer = setTimeout(() => (current.count = 0), AUTO_DISMISS_MS);
      const title =
        current.count === 1
          ? t("toast.uploaded", { name: file.name })
          : t("toast.uploadedMany", { count: current.count });
      notify.success(title, undefined, `${UPLOADS_TOAST_ID}-${kbId}`);
      // The header's file count comes from the server.
      router.refresh();
    },
    (file, error) => {
      // The storage leg has no core response to show, so its failure is announced with the file's name (P-F17);
      // the other steps answer with a catalog code (and a request id for 5xx) that the shared error toast maps.
      if (error instanceof StorageUploadError) notify.problem(file.name, storageText(error));
      else notify.error(error);
    },
  );
  const requestId = (id: string) => tErr("requestId", { id });
  const empty = list.files.length === 0 && queue.items.length === 0;
  return (
    <div className={`${PAGE_COLUMN} pb-16`}>
      <UploadDropZone onFiles={queue.add} />
      {list.pollError !== undefined && (
        <div className="mt-4">
          <InlineAlert title={describeApiError(list.pollError, t("errors.pollFailed"), requestId)} />
        </div>
      )}
      {queue.items.length > 0 && (
        <ul aria-label={t("uploadsLabel")} className="border-border mt-6 border-t">
          {queue.items.map((item) => (
            <UploadRow
              key={item.id}
              item={item}
              onCancel={() => queue.cancel(item.id)}
              onRetry={() => queue.retry(item.id)}
              onDismiss={() => queue.dismiss(item.id)}
            />
          ))}
        </ul>
      )}
      {empty ? (
        <EmptyState line="kb" level="h2" title={t("emptyTitle")} body={t("emptyBody")} />
      ) : (
        list.files.length > 0 && (
          <ul aria-label={t("fileListLabel")} className="border-border mt-6 border-t">
            {list.files.map((file) => (
              <FileRow
                key={file.id}
                orgId={orgId}
                kbId={kbId}
                file={file}
                onChange={list.upsert}
                onRemove={list.remove}
              />
            ))}
          </ul>
        )
      )}
      {list.moreError !== undefined && (
        <div className="mt-4">
          <InlineAlert title={describeApiError(list.moreError, t("errors.loadMoreFailed"), requestId)} />
        </div>
      )}
      {list.cursor && (
        <div className="mt-6 flex justify-center">
          <Button
            variant="outline"
            onClick={() => void list.loadMore()}
            pending={list.loadingMore}
            disabled={list.loadingMore}
          >
            {t("loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}
