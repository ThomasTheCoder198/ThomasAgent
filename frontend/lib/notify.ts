import { createElement, type ReactNode } from "react";
import { toast } from "sonner";

import type { Locale } from "@/i18n/config";
import { ApiError, networkError } from "@/lib/api/client";
import { errorDefinitions } from "@/lib/errors/codes.gen";

/** Confirmations leave on their own; an error stays until dismissed, because reading it must not race a timer. */
export const AUTO_DISMISS_MS = 4000;
const HTTP_SERVER_ERROR = 500;

export type ErrorContext = {
  locale: Locale;
  /** The localized "Request ID" label; the id itself is shown in mono next to it. */
  requestIdLabel: string;
};

export type ErrorNotice = { title: string; requestId?: string };

/**
 * What to tell the user about a failure: the catalog message for the code in their language (never a raw
 * network string), and the request id only for 5xx, where the operator needs it to find the log line.
 */
export function describeError(error: unknown, locale: Locale): ErrorNotice {
  const failure = error instanceof ApiError ? error : networkError(locale);
  const title = errorDefinitions[failure.code]?.message[locale] ?? failure.message;
  const isServerError = failure.status >= HTTP_SERVER_ERROR;
  return { title, requestId: isServerError ? failure.requestId : undefined };
}

export function notifySuccess(title: string, description?: ReactNode): string | number {
  return toast.success(title, { description, duration: AUTO_DISMISS_MS });
}

export function notifyInfo(title: string, description?: ReactNode): string | number {
  return toast.info(title, { description, duration: AUTO_DISMISS_MS });
}

/**
 * A failure with caller-supplied words (a storage upload has no core code to map). Title and detail sit in one
 * role="alert" node: Sonner's own live region is polite, and an error must interrupt, announced whole.
 */
export function notifyProblem(title: string, detail?: ReactNode): string | number {
  const notice = createElement(
    "span",
    { role: "alert", className: "flex flex-col gap-0.5" },
    title,
    detail
      ? createElement("span", { className: "text-muted-foreground text-[0.8125rem] font-normal" }, detail)
      : null,
  );
  return toast.error(notice, { duration: Infinity, dismissible: true });
}

export function notifyError(error: unknown, { locale, requestIdLabel }: ErrorContext): string | number {
  const { title, requestId } = describeError(error, locale);
  const detail = requestId
    ? createElement(
        "span",
        null,
        `${requestIdLabel}: `,
        createElement("span", { className: "tabular" }, requestId),
      )
    : undefined;
  return notifyProblem(title, detail);
}
