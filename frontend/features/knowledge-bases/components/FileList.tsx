"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { EmptyState } from "@/features/shell/EmptyState";
import { describeApiError } from "@/lib/api/client";
import type { KbFile } from "@/lib/ragx/types";

import { useFiles } from "../useFiles";
import { useUploadQueue } from "../useUploadQueue";
import { FileRow } from "./FileRow";
import { UploadDropZone } from "./UploadDropZone";
import { UploadRow } from "./UploadRow";

type Props = { orgId: string; kbId: string; initialFiles: KbFile[]; initialCursor: string | null };

/** Upload rows first (trains leaving), then the documents; the list follows ingest while anything is in flight. */
export function FileList({ orgId, kbId, initialFiles, initialCursor }: Props) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const router = useRouter();
  const list = useFiles(orgId, kbId, initialFiles, initialCursor);
  const queue = useUploadQueue(orgId, kbId, (file) => {
    list.upsert(file);
    // The sign's file count comes from the server.
    router.refresh();
  });
  const requestId = (id: string) => tErr("requestId", { id });
  const empty = list.files.length === 0 && queue.items.length === 0;
  return (
    <div className="mx-auto w-full max-w-272 px-4 pb-16 md:px-8">
      <UploadDropZone onFiles={queue.add} />
      {list.pollError !== undefined && (
        <div className="mt-4">
          <InlineAlert title={describeApiError(list.pollError, t("errors.pollFailed"), requestId)} />
        </div>
      )}
      {queue.items.length > 0 && (
        <ul aria-label={t("uploadsLabel")}>
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
        <ul aria-label={t("fileListLabel")}>
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
