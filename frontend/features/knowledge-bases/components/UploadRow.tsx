"use client";

import { RotateCw, TriangleAlert, X } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { describeApiError } from "@/lib/api/client";
import { PERCENT, formatBytes, formatPercent } from "@/lib/format";

import { StorageUploadError } from "../upload";
import type { QueueItem } from "../useUploadQueue";
import { useStorageFailureText } from "./useStorageFailureText";

type Props = { item: QueueItem; onCancel: () => void; onRetry: () => void; onDismiss: () => void };

function Progress({ item }: { item: QueueItem }) {
  const t = useTranslations("kb");
  const locale = useLocale();
  const ratio = Math.min(1, item.loaded / item.file.size);
  return (
    <span className="flex min-w-0 items-center gap-3">
      <span
        role="progressbar"
        aria-label={t("uploadProgress", { name: item.file.name })}
        aria-valuemin={0}
        aria-valuemax={PERCENT}
        aria-valuenow={Math.round(ratio * PERCENT)}
        className="bg-border h-1 min-w-24 flex-1 overflow-hidden rounded-full"
      >
        <span className="bg-primary block h-full rounded-full" style={{ width: `${ratio * PERCENT}%` }} />
      </span>
      <span className="tabular text-muted-foreground w-10 shrink-0 text-right text-xs">
        {formatPercent(ratio, locale)}%
      </span>
    </span>
  );
}

/** What went wrong for a core step, in core's words with its request id. */
function useApiFailureText(item: QueueItem): ReactNode {
  const tErr = useTranslations("errors");
  return describeApiError(item.error, tErr("generic"), (id) => tErr("requestId", { id }));
}

/**
 * A file that fails the local check (type, size, empty) never reached the network, so nothing toasted it and
 * the row announces it as an alert. Every other failure was already announced by its toast (role=alert), so the
 * row states it quietly beside Retry: a second alert would make screen readers say it twice.
 */
function Failure({ item }: { item: QueueItem }) {
  const t = useTranslations("kb");
  const storageText = useStorageFailureText();
  const apiText = useApiFailureText(item);
  if (item.problem) return <InlineAlert title={t(`problem.${item.problem}`)} />;
  return (
    <p className="flex gap-2 text-[0.8125rem]">
      <TriangleAlert aria-hidden className="text-destructive mt-0.5 size-4 shrink-0" />
      <span className="min-w-0">
        {item.error instanceof StorageUploadError ? storageText(item.error) : apiText}
      </span>
    </p>
  );
}

/** One file on its way: queued, then three phases (Preparing, byte progress, Finishing); failures stay until dismissed. */
export function UploadRow({ item, onCancel, onRetry, onDismiss }: Props) {
  const t = useTranslations("kb");
  const locale = useLocale();
  const phase = {
    queued: <span className="text-muted-foreground text-[0.8125rem]">{t("queued")}</span>,
    preparing: <span className="text-muted-foreground text-[0.8125rem]">{t("preparing")}</span>,
    uploading: <Progress item={item} />,
    confirming: <span className="text-muted-foreground text-[0.8125rem]">{t("confirming")}</span>,
    failed: null,
  }[item.state];
  const failed = item.state === "failed";
  return (
    <li className="border-border grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 border-b px-3 py-3">
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[0.9375rem] font-semibold" title={item.file.name}>
          {item.file.name}
        </span>
        <span className="tabular text-muted-foreground shrink-0 text-xs">
          {formatBytes(item.file.size, locale)}
        </span>
      </span>
      <span className="col-start-2 row-span-2 row-start-1 flex items-center justify-end gap-2">
        {failed && !item.problem && (
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RotateCw aria-hidden />
            {t("retry")}
          </Button>
        )}
        {failed && (
          <Button variant="ghost" size="sm" onClick={onDismiss}>
            {t("dismiss")}
          </Button>
        )}
        {/* No Cancel once confirm is in flight: ragx may have committed, and an abort would hide a stored file. */}
        {!failed && item.state !== "confirming" && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("cancel")}
            title={t("cancel")}
            onClick={onCancel}
          >
            <X aria-hidden />
          </Button>
        )}
      </span>
      {phase && <span className="col-start-1 min-w-0">{phase}</span>}
      {failed && (
        <div className="col-start-1">
          <Failure item={item} />
        </div>
      )}
    </li>
  );
}
