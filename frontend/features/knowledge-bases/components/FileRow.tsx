"use client";

import { Download, MoreHorizontal, RotateCw, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useRef, useState, type RefObject } from "react";

import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import type { Locale } from "@/i18n/config";
import { formatBytes, formatDateTime } from "@/lib/format";
import type { KbFile } from "@/lib/ragx/types";
import { useNotify } from "@/lib/useNotify";

import { deleteFile, retryFile } from "../api";
import { fetchDownloadUrl } from "../upload";
import { ConfirmDelete } from "./ConfirmDelete";
import { FileStatusBadge } from "./FileStatusBadge";

type Props = {
  orgId: string;
  kbId: string;
  file: KbFile;
  onChange: (file: KbFile) => void;
  onRemove: (fileId: string) => void;
};

function useFileActions({ orgId, kbId, file, onChange, onRemove }: Props) {
  const t = useTranslations("kb");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [busy, setBusy] = useState<"download" | "retry" | null>(null);

  async function run(kind: "download" | "retry", action: () => Promise<void>) {
    setBusy(kind);
    try {
      await action();
    } catch (err) {
      notify.error(err);
    } finally {
      setBusy(null);
    }
  }

  return {
    busy,
    // The link is presigned for an attachment download, so navigating to it saves the file and keeps this page.
    download: () =>
      run("download", async () => {
        const { url } = await fetchDownloadUrl(orgId, kbId, file.id, locale);
        window.location.assign(url);
      }),
    retry: () =>
      run("retry", async () => {
        onChange(await retryFile(orgId, kbId, file.id, locale));
        notify.success(t("toast.retried", { name: file.name }));
      }),
    remove: async () => {
      await deleteFile(orgId, kbId, file.id, locale);
      onRemove(file.id);
      router.refresh();
    },
  };
}

type Actions = ReturnType<typeof useFileActions>;

/** From md the three actions sit inline as icon buttons; below md they fold into one menu. */
function RowActions({
  file,
  actions,
  menuTrigger,
  onDelete,
}: {
  file: KbFile;
  actions: Actions;
  menuTrigger: RefObject<HTMLButtonElement | null>;
  onDelete: (fromMenu: boolean) => void;
}) {
  const t = useTranslations("kb");
  const failed = file.status === "failed";
  return (
    <>
      <span className="flex items-center justify-end max-md:hidden">
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
        {failed && (
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
          onClick={() => onDelete(false)}
        >
          <Trash2 aria-hidden />
        </Button>
      </span>
      <span className="flex items-center justify-end md:hidden">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              ref={menuTrigger}
              variant="ghost"
              size="icon-sm"
              aria-label={t("fileActions", { name: file.name })}
              pending={actions.busy !== null}
            >
              <MoreHorizontal aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => void actions.download()}>
              <Download aria-hidden />
              {t("download")}
            </DropdownMenuItem>
            {failed && (
              <DropdownMenuItem onSelect={() => void actions.retry()}>
                <RotateCw aria-hidden />
                {t("retry")}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem variant="destructive" onSelect={() => onDelete(true)}>
              <Trash2 aria-hidden />
              {t("delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    </>
  );
}

/** A ruled row; below `md` it stacks: name and status first, then size and date, with the actions at the end. */
export function FileRow(props: Props) {
  const { file } = props;
  const t = useTranslations("kb");
  const locale = useLocale();
  const actions = useFileActions(props);
  const [confirming, setConfirming] = useState(false);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  /** The menu closes with its item, so the dialog must hand focus back to the menu's button; inline it finds its opener. */
  const returnFocus = useRef<HTMLElement | null>(null);
  return (
    <li className="border-border grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 border-b px-3 py-3 md:grid-cols-[minmax(0,1fr)_minmax(0,14rem)_5rem_9rem_auto] md:gap-x-6">
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="truncate text-[0.9375rem] font-semibold" title={file.name}>
          {file.name}
        </span>
        <span
          className="tabular text-muted-foreground shrink-0 text-xs"
          title={t("version", { version: file.version })}
        >
          v{file.version}
        </span>
      </span>
      <FileStatusBadge status={file.status} errorCode={file.errorCode} />
      <span className="text-muted-foreground tabular col-start-1 flex min-w-0 flex-wrap gap-x-3 text-xs md:contents">
        <span>{formatBytes(file.sizeBytes, locale)}</span>
        <span>{formatDateTime(file.updatedAt, locale)}</span>
      </span>
      <span className="col-start-2 row-span-3 row-start-1 md:col-start-auto md:row-span-1 md:row-start-auto">
        <RowActions
          file={file}
          actions={actions}
          menuTrigger={menuTrigger}
          onDelete={(fromMenu) => {
            returnFocus.current = fromMenu ? menuTrigger.current : null;
            setConfirming(true);
          }}
        />
      </span>
      {confirming && (
        <ConfirmDelete
          onOpenChange={setConfirming}
          title={t("deleteTitle", { name: file.name })}
          body={t("deleteFileBody")}
          successMessage={t("toast.deleted", { name: file.name })}
          returnFocusRef={returnFocus}
          onConfirm={actions.remove}
        />
      )}
    </li>
  );
}
