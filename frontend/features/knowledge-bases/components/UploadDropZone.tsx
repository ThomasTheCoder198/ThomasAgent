"use client";

import { Upload } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type ChangeEvent } from "react";

import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

import { UPLOAD_ACCEPT } from "../upload-rules";

const hasFiles = (event: DragEvent) => event.dataTransfer?.types.includes("Files") ?? false;

/** Any file dragged over the page counts: while dragging, the drop zone and an outline around the window light up (Escape clears it). */
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

/** A dashed drop target with a Choose files button; dropping anywhere on the page works too. */
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
    <div
      className={cn(
        "border-input flex flex-col items-center gap-3 rounded-sm border border-dashed px-4 py-8 text-center transition-colors",
        dragging && "border-primary bg-accent",
      )}
    >
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
      <Upload aria-hidden className="text-muted-foreground size-6" />
      <div>
        <p className="text-sm font-medium">{dragging ? t("dropHere") : t("dropTitle")}</p>
        <p className="text-muted-foreground mt-1 text-[0.8125rem]">{t("dropHint")}</p>
      </div>
      <Button variant="outline" onClick={() => input.current?.click()}>
        {t("chooseFiles")}
      </Button>
      {dragging && (
        <div aria-hidden className="pointer-events-none fixed inset-0 z-30 p-4">
          <div className="border-primary size-full rounded-sm border-2 border-dashed" />
        </div>
      )}
    </div>
  );
}
