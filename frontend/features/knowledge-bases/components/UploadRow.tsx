"use client";

import { RotateCw, X } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { ReactNode } from "react";

import { IconButton } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { describeApiError } from "@/lib/api/client";
import { PERCENT, formatBytes, formatPercent } from "@/lib/format";

import { NETWORK_CODE, StorageUploadError } from "../upload";
import type { QueueItem } from "../useUploadQueue";

const ACTION = "size-touch md:size-8.5";

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
        className="bg-rule-strong h-0.5 min-w-24 flex-1 overflow-hidden rounded-full"
      >
        <span className="bg-ink block h-full" style={{ width: `${ratio * PERCENT}%` }} />
      </span>
      <span className="tabular text-ink-2 w-10 shrink-0 text-right text-xs">
        {formatPercent(ratio, locale)}%
      </span>
    </span>
  );
}

/** What went wrong, in the failing step's own terms: the file itself, storage (an S3 code, no request id), or core. */
function useFailureText(item: QueueItem): ReactNode {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  if (item.problem) return t(`problem.${item.problem}`);
  if (item.error instanceof StorageUploadError) {
    return t.rich(item.error.code === NETWORK_CODE ? "problem.storageUnreachable" : "problem.storage", {
      code: item.error.code,
      mono: (chunks) => <span className="tabular">{chunks}</span>,
    });
  }
  return describeApiError(item.error, tErr("generic"), (id) => tErr("requestId", { id }));
}

function Failure({ item }: { item: QueueItem }) {
  const text = useFailureText(item);
  return <InlineAlert title={text} />;
}

/** One file on its way: queued, then three phases (Preparing, byte progress, Finishing); failures stay until dismissed. */
export function UploadRow({ item, onCancel, onRetry, onDismiss }: Props) {
  const t = useTranslations("kb");
  const locale = useLocale();
  const phase = {
    queued: <span className="text-ink-3 text-[13px]">{t("queued")}</span>,
    preparing: <span className="text-ink-2 text-[13px]">{t("preparing")}</span>,
    uploading: <Progress item={item} />,
    confirming: <span className="text-ink-2 text-[13px]">{t("confirming")}</span>,
    failed: null,
  }[item.state];
  const failed = item.state === "failed";
  return (
    <li className="border-rule-2 grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-5 gap-y-2 border-b px-3 py-3">
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[15px] font-semibold" title={item.file.name}>
          {item.file.name}
        </span>
        <span className="tabular text-ink-3 shrink-0 text-xs">{formatBytes(item.file.size, locale)}</span>
      </span>
      <span className="row-span-2 flex justify-end">
        {failed && !item.problem && (
          <IconButton label={t("retry")} className={ACTION} onClick={onRetry}>
            <RotateCw aria-hidden className="size-4" />
          </IconButton>
        )}
        {/* No Cancel once confirm is in flight: ragx may have committed, and an abort would hide a stored file. */}
        {item.state !== "confirming" && (
          <IconButton
            label={failed ? t("dismiss") : t("cancel")}
            className={ACTION}
            onClick={failed ? onDismiss : onCancel}
          >
            <X aria-hidden className="size-4" />
          </IconButton>
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
