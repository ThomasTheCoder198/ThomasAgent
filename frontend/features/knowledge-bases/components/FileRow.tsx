"use client";

import { Download, RotateCw, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import type { Locale } from "@/i18n/config";
import { describeApiError } from "@/lib/api/client";
import { formatBytes, formatDateTime } from "@/lib/format";
import type { KbFile } from "@/lib/ragx/types";

import { deleteFile, retryFile } from "../api";
import { fetchDownloadUrl } from "../upload";
import { ConfirmDelete } from "./ConfirmDelete";
import { FileStatusMark } from "./FileStatusMark";

type Props = {
  orgId: string;
  kbId: string;
  file: KbFile;
  onChange: (file: KbFile) => void;
  onRemove: (fileId: string) => void;
};

function useFileActions({ orgId, kbId, file, onChange, onRemove }: Props) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [busy, setBusy] = useState<"download" | "retry" | null>(null);
  const [failure, setFailure] = useState<string>();

  async function run(kind: "download" | "retry", fallback: string, action: () => Promise<void>) {
    setBusy(kind);
    setFailure(undefined);
    try {
      await action();
    } catch (err) {
      setFailure(describeApiError(err, fallback, (id) => tErr("requestId", { id })));
    } finally {
      setBusy(null);
    }
  }

  return {
    busy,
    failure,
    // The link is presigned for an attachment download, so navigating to it saves the file and keeps this page.
    download: () =>
      run("download", t("downloadFailed"), async () => {
        const { url } = await fetchDownloadUrl(orgId, kbId, file.id, locale);
        window.location.assign(url);
      }),
    retry: () =>
      run("retry", t("errors.retryFailed"), async () =>
        onChange(await retryFile(orgId, kbId, file.id, locale)),
      ),
    remove: async () => {
      await deleteFile(orgId, kbId, file.id, locale);
      onRemove(file.id);
      router.refresh();
    },
  };
}

/** A ruled row; below `md` it stacks: name and status first, then size, date and the actions. */
export function FileRow(props: Props) {
  const { file } = props;
  const t = useTranslations("kb");
  const locale = useLocale();
  const actions = useFileActions(props);
  const [confirming, setConfirming] = useState(false);
  return (
    <li className="border-border grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-5 gap-y-1 border-b px-3 py-3 md:grid-cols-[minmax(0,1fr)_minmax(0,12rem)_5rem_9rem_auto]">
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[15px] font-semibold" title={file.name}>
          {file.name}
        </span>
        <span
          className="tabular text-muted-foreground shrink-0 text-xs"
          title={t("version", { version: file.version })}
        >
          v{file.version}
        </span>
      </span>
      <FileStatusMark status={file.status} errorCode={file.errorCode} />
      <span className="text-muted-foreground tabular col-start-1 flex min-w-0 flex-wrap gap-x-3 text-xs md:contents">
        <span className="md:text-muted-foreground">{formatBytes(file.sizeBytes, locale)}</span>
        <span>{formatDateTime(file.updatedAt, locale)}</span>
      </span>
      <span className="col-start-2 row-start-2 flex justify-end md:col-start-auto md:row-start-auto">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t("download")}
          title={t("download")}
          pending={actions.busy === "download"}
          onClick={() => void actions.download()}
        >
          <Download aria-hidden />
        </Button>
        {file.status === "failed" && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("retry")}
            title={t("retry")}
            pending={actions.busy === "retry"}
            onClick={() => void actions.retry()}
          >
            <RotateCw aria-hidden />
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t("delete")}
          title={t("delete")}
          onClick={() => setConfirming(true)}
        >
          <Trash2 aria-hidden />
        </Button>
      </span>
      {actions.failure && (
        <div className="col-span-full mt-2">
          <InlineAlert title={actions.failure} />
        </div>
      )}
      {confirming && (
        <ConfirmDelete
          onOpenChange={setConfirming}
          title={t("deleteTitle", { name: file.name })}
          body={t("deleteFileBody")}
          onConfirm={actions.remove}
        />
      )}
    </li>
  );
}
