"use client";

import { Info, TriangleAlert, X } from "lucide-react";
import { useTranslations } from "next-intl";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { IconButton } from "./Button";

export type ToastTone = "info" | "error";
/** The title is plain text because it also names the dismiss button. */
export type ToastInput = { tone: ToastTone; title: string; detail?: ReactNode };
type Toast = ToastInput & { id: number };
type ToastApi = { show: (input: ToastInput) => void; dismiss: (id: number) => void };

/** Info leaves on its own; an error stays until dismissed, because reading it must not race a timer. */
export const INFO_TOAST_MS = 6000;
/** Older toasts give way, so a burst of failures cannot cover the page. */
export const MAX_TOASTS = 3;

/** Marks the toast stack, so a modal layer can tell a click on a toast from a click outside it. */
const TOAST_REGION_ATTRIBUTE = "data-toast-region";

/** True when an event target is inside the toast stack; modal dialogs must not close for those clicks. */
export function isInToastRegion(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(`[${TOAST_REGION_ATTRIBUTE}]`) !== null;
}

const ToastContext = createContext<ToastApi | null>(null);

export function useToast(): ToastApi {
  const api = useContext(ToastContext);
  if (!api) throw new Error("useToast needs a ToastProvider");
  return api;
}

/**
 * Lifted notices over the page, bottom right (bottom, full width on phones). Both live regions are in the DOM
 * before any toast, so screen readers announce what is added: errors in role=alert, info in role=status.
 * The explicit aria-live keeps them out of a modal's aria-hidden (Radix spares [aria-live] nodes), and
 * aria-atomic=false reads only the new toast instead of every one still showing.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());
  const clearTimer = useCallback((id: number) => {
    clearTimeout(timers.current.get(id));
    timers.current.delete(id);
  }, []);
  const dismiss = useCallback(
    (id: number) => {
      clearTimer(id);
      setToasts((all) => all.filter((toast) => toast.id !== id));
    },
    [clearTimer],
  );
  const show = useCallback(
    (input: ToastInput) => {
      const id = nextId.current++;
      setToasts((all) => {
        const kept = [...all, { ...input, id }];
        kept.slice(0, -MAX_TOASTS).forEach((gone) => clearTimer(gone.id));
        return kept.slice(-MAX_TOASTS);
      });
      if (input.tone === "info")
        timers.current.set(
          id,
          setTimeout(() => dismiss(id), INFO_TOAST_MS),
        );
    },
    [clearTimer, dismiss],
  );
  useEffect(() => {
    const pending = timers.current;
    return () => {
      pending.forEach(clearTimeout);
      pending.clear();
    };
  }, []);
  const api = useMemo(() => ({ show, dismiss }), [show, dismiss]);
  const of = (tone: ToastTone) =>
    toasts
      .filter((toast) => toast.tone === tone)
      .map((toast) => (
        <ToastItem
          key={toast.id}
          toast={toast}
          onDismiss={(item) => {
            keepFocus(item);
            dismiss(toast.id);
          }}
        />
      ));
  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        {...{ [TOAST_REGION_ATTRIBUTE]: "" }}
        className="pointer-events-none fixed inset-x-4 bottom-[max(1rem,env(safe-area-inset-bottom))] z-50 md:inset-x-auto md:right-6 md:bottom-6 md:w-96"
      >
        <div
          role="alert"
          aria-live="assertive"
          aria-atomic="false"
          tabIndex={-1}
          className="flex flex-col gap-2 outline-none"
        >
          {of("error")}
        </div>
        <div
          role="status"
          aria-live="polite"
          aria-atomic="false"
          tabIndex={-1}
          className="flex flex-col gap-2 outline-none not-empty:mt-2"
        >
          {of("info")}
        </div>
      </div>
    </ToastContext.Provider>
  );
}

/**
 * When the toast that held focus goes away, focus moves to the next toast's dismiss button, else to its region,
 * so keyboard users are not dropped on <body>.
 */
function keepFocus(item: HTMLElement | null) {
  if (!item?.contains(document.activeElement)) return;
  const sibling = item.nextElementSibling ?? item.previousElementSibling;
  const next = sibling?.querySelector<HTMLElement>("button") ?? item.parentElement;
  next?.focus();
}

function ToastItem({ toast, onDismiss }: { toast: Toast; onDismiss: (item: HTMLElement | null) => void }) {
  const t = useTranslations("common");
  const item = useRef<HTMLDivElement>(null);
  const Icon = toast.tone === "error" ? TriangleAlert : Info;
  return (
    <div
      ref={item}
      className="bg-sign text-sign-ink shadow-lift animate-rise pointer-events-auto flex items-start gap-3 rounded-xl py-2 pr-2 pl-4 text-sm"
    >
      <Icon
        aria-hidden
        className={toast.tone === "error" ? "text-app mt-2.5 size-4 shrink-0" : "mt-2.5 size-4 shrink-0"}
      />
      <div className="min-w-0 flex-1 py-2">
        <p className="truncate font-semibold">{toast.title}</p>
        {toast.detail && <p className="text-sign-ink-2 mt-0.5">{toast.detail}</p>}
      </div>
      <IconButton
        tone="sign"
        label={t("dismissNotification", { title: toast.title })}
        onClick={() => onDismiss(item.current)}
      >
        <X aria-hidden className="size-4" />
      </IconButton>
    </div>
  );
}
