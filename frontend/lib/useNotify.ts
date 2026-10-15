"use client";

import { useLocale, useTranslations } from "next-intl";
import { useMemo } from "react";

import type { Locale } from "@/i18n/config";

import { notifyError, notifyInfo, notifyProblem, notifySuccess } from "./notify";

/** `notify` bound to the current locale, so a component reports an outcome without handling translation. */
export function useNotify() {
  const locale = useLocale() as Locale;
  const t = useTranslations("common");
  const requestIdLabel = t("requestIdLabel");
  return useMemo(
    () => ({
      success: notifySuccess,
      info: notifyInfo,
      problem: notifyProblem,
      error: (error: unknown) => notifyError(error, { locale, requestIdLabel }),
    }),
    [locale, requestIdLabel],
  );
}
