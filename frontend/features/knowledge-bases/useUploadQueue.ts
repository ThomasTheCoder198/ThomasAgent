"use client";

import { useLocale } from "next-intl";
import { useCallback, useEffect, useRef, useState } from "react";

import type { Locale } from "@/i18n/config";
import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KbFile, UploadTicket } from "@/lib/ragx/types";

import { StorageUploadError, confirmUpload, postToStorage, presignUpload } from "./upload";
import { checkUpload, type UploadProblem } from "./upload-rules";

export type UploadStep = "presign" | "storage" | "confirm";
export type QueueItem = {
  id: string;
  file: File;
  state: "queued" | "preparing" | "uploading" | "confirming" | "failed";
  loaded: number;
  ticket?: UploadTicket;
  failedStep?: UploadStep;
  problem?: UploadProblem;
  error?: unknown;
};

type Outcome = { ok: true; file: KbFile } | { ok: false; step: UploadStep; error: unknown };
type RunContext = {
  orgId: string;
  kbId: string;
  locale: Locale;
  signal: AbortSignal;
  patch: (change: Partial<QueueItem>) => void;
};

const IN_PROGRESS: ReadonlySet<QueueItem["state"]> = new Set(["preparing", "uploading", "confirming"]);
/** Confirm answers these on field `file` when the bytes are not (or not fully) in storage: post them again. */
const RESEND_CODES: ReadonlySet<string> = new Set(["MISSING", "SIZE_MISMATCH"]);
const FILE_FIELD = "file";
const HTTP_FORBIDDEN = 403;

const isExpired = (ticket: UploadTicket) => Date.parse(ticket.expiresAt) <= Date.now();
const isAbort = (error: unknown) => error instanceof Error && error.name === "AbortError";

function confirmWantsResend(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.code !== ErrorCode.VALIDATION_FAILED) return false;
  const fields = error.details?.fields as { field: string; code: string }[] | undefined;
  return !!fields?.some((f) => f.field === FILE_FIELD && RESEND_CODES.has(f.code));
}

function failedOutcome(error: unknown, step: UploadStep, patch: RunContext["patch"]): Outcome {
  // A 403 (AccessDenied: clock skew, signature) would repeat with the same ticket, so Retry must presign again.
  if (error instanceof StorageUploadError && error.status === HTTP_FORBIDDEN) patch({ ticket: undefined });
  return { ok: false, step: confirmWantsResend(error) ? "storage" : step, error };
}

/**
 * One pass over the steps still to do: presign (only without a usable ticket), post the bytes (unless only
 * confirm is left), confirm. A confirm retry reuses its ticket's file id even after the ticket expired, because
 * a new presign would name a different file whose bytes were never posted.
 */
async function runItem(item: QueueItem, ctx: RunContext): Promise<Outcome> {
  const { orgId, kbId, locale, signal, patch } = ctx;
  const confirmOnly = item.failedStep === "confirm" && item.ticket !== undefined;
  let step: UploadStep = "presign";
  try {
    let ticket = item.ticket;
    if (!ticket || (!confirmOnly && isExpired(ticket))) {
      patch({ state: "preparing", loaded: 0 });
      ticket = await presignUpload(orgId, kbId, item.file, locale, signal);
      patch({ ticket });
    }
    if (!confirmOnly) {
      step = "storage";
      patch({ state: "uploading", loaded: 0 });
      await postToStorage(ticket, item.file, (loaded) => patch({ loaded }), signal);
    }
    step = "confirm";
    patch({ state: "confirming" });
    return { ok: true, file: await confirmUpload(orgId, kbId, ticket.fileId, locale, signal) };
  } catch (error) {
    if (signal.aborted || isAbort(error)) throw error;
    return failedOutcome(error, step, patch);
  }
}

type Controls = {
  controllers: Map<string, AbortController>;
  patch: (id: string, change: Partial<QueueItem>) => void;
  remove: (id: string) => void;
  uploaded: (file: KbFile) => void;
};

/** Starts one queued item. The controller doubles as the started-ids guard: a strict-mode rerun finds the id taken. */
function startItem(next: QueueItem, base: Omit<RunContext, "signal" | "patch">, controls: Controls) {
  const { controllers, patch, remove, uploaded } = controls;
  const controller = new AbortController();
  controllers.set(next.id, controller);
  patch(next.id, { state: "preparing", loaded: 0, error: undefined });
  const ctx = {
    ...base,
    signal: controller.signal,
    patch: (change: Partial<QueueItem>) => patch(next.id, change),
  };
  runItem(next, ctx)
    .then((outcome) => {
      if (!outcome.ok)
        return patch(next.id, { state: "failed", failedStep: outcome.step, error: outcome.error });
      remove(next.id);
      uploaded(outcome.file);
    })
    .catch(() => remove(next.id))
    .finally(() => controllers.delete(next.id));
}

function toQueueItem(file: File): QueueItem {
  const problem = checkUpload(file) ?? undefined;
  return { id: crypto.randomUUID(), file, state: problem ? "failed" : "queued", loaded: 0, problem };
}

/** Uploads one file at a time: 50 MB files in parallel would starve each other and the rest of the app. */
export function useUploadQueue(orgId: string, kbId: string, onUploaded: (file: KbFile) => void) {
  const locale = useLocale() as Locale;
  const [items, setItems] = useState<QueueItem[]>([]);
  const controllers = useRef(new Map<string, AbortController>());
  const onUploadedRef = useRef(onUploaded);
  useEffect(() => {
    onUploadedRef.current = onUploaded;
  });

  // Leaving the page (or switching KB, which remounts this) must stop the bytes: nothing would show the outcome.
  useEffect(() => {
    const running = controllers.current;
    return () => {
      for (const controller of running.values()) controller.abort();
    };
  }, []);

  const patch = useCallback((id: string, change: Partial<QueueItem>) => {
    setItems((all) => all.map((item) => (item.id === id ? { ...item, ...change } : item)));
  }, []);
  const remove = useCallback((id: string) => setItems((all) => all.filter((item) => item.id !== id)), []);
  const add = useCallback(
    (files: Iterable<File>) => setItems((all) => [...all, ...[...files].map(toQueueItem)]),
    [],
  );

  useEffect(() => {
    if (items.some((item) => IN_PROGRESS.has(item.state))) return;
    const next = items.find((item) => item.state === "queued");
    if (!next || controllers.current.has(next.id)) return;
    const uploaded = (file: KbFile) => onUploadedRef.current(file);
    startItem(next, { orgId, kbId, locale }, { controllers: controllers.current, patch, remove, uploaded });
  }, [items, kbId, locale, orgId, patch, remove]);

  const cancel = useCallback(
    (id: string) => {
      const controller = controllers.current.get(id);
      if (controller) controller.abort();
      else remove(id);
    },
    [remove],
  );
  const retry = useCallback((id: string) => patch(id, { state: "queued", error: undefined }), [patch]);

  return { items, add, cancel, retry, dismiss: cancel };
}
