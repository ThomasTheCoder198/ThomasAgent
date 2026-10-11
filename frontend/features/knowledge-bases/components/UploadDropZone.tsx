"use client";

import { Upload } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type ChangeEvent } from "react";

import { Button } from "@/components/ui/Button";

import { UPLOAD_ACCEPT } from "../upload-rules";

const hasFiles = (event: DragEvent) => event.dataTransfer?.types.includes("Files") ?? false;

/** Any file dragged over the page counts: while dragging, a dashed outline covers the window (Escape clears it). */
function useWindowDrop(onFiles: (files: File[]) => void) {
  const [dragging, setDragging] = useState(false);
  const onFilesRef = useRef(onFiles);
  useEffect(() => {
    onFilesRef.current = onFiles;
  });
  useEffect(() => {
    let depth = 0;
    const enter = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      depth += 1;
      setDragging(true);
    };
    const over = (e: DragEvent) => hasFiles(e) && e.preventDefault();
    const leave = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) setDragging(false);
    };
    const drop = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
      depth = 0;
      setDragging(false);
      onFilesRef.current(Array.from(e.dataTransfer?.files ?? []));
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      depth = 0;
      setDragging(false);
    };
    const handlers = { dragenter: enter, dragover: over, dragleave: leave, drop, keydown: escape } as const;
    for (const [name, handler] of Object.entries(handlers))
      window.addEventListener(name, handler as EventListener);
    return () => {
      for (const [name, handler] of Object.entries(handlers))
        window.removeEventListener(name, handler as EventListener);
    };
  }, []);
  return dragging;
}

export function UploadDropZone({ onFiles }: { onFiles: (files: File[]) => void }) {
  const t = useTranslations("kb");
  const input = useRef<HTMLInputElement>(null);
  const dragging = useWindowDrop(onFiles);

  function picked(event: ChangeEvent<HTMLInputElement>) {
    onFiles(Array.from(event.target.files ?? []));
    // The same file can be picked again after a failure.
    event.target.value = "";
  }

  return (
    <div className="border-rule flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-3 py-4">
      <input
        ref={input}
        type="file"
        multiple
        accept={UPLOAD_ACCEPT}
        onChange={picked}
        className="sr-only"
        tabIndex={-1}
        aria-hidden
      />
      <Button onClick={() => input.current?.click()} className="min-h-touch md:min-h-0">
        <Upload aria-hidden className="size-4" />
        {t("upload")}
      </Button>
      <span className="text-ink-3 text-[13px]">{t("dropHint")}</span>
      {dragging && (
        <div
          aria-hidden
          className="bg-paper/85 pointer-events-none fixed inset-0 z-30 grid place-items-center p-4"
        >
          <div className="border-rule-strong text-ink grid size-full place-items-center rounded-xl border-2 border-dashed text-lg font-semibold">
            {t("dropHere")}
          </div>
        </div>
      )}
    </div>
  );
}
