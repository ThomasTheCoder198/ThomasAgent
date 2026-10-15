"use client";

import { CircleCheckIcon, InfoIcon, Loader2Icon, OctagonXIcon, TriangleAlertIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { useTheme } from "next-themes";
import type { CSSProperties } from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";

import { useIsBelowMd } from "@/hooks/useMediaQuery";

/** Older toasts give way, so a burst of failures cannot cover the page. */
export const MAX_VISIBLE_TOASTS = 3;

/**
 * Sonner ships an unlayered stylesheet, which beats Tailwind's layered utilities whatever the specificity,
 * so every override of one of its own declarations carries `!`. The toast is a floating layer: card fill,
 * hairline border, lift shadow, 14px radius. Its stack takes pointer events back because a modal layer sets
 * `pointer-events: none` on <body>.
 */
const TOAST_VARS = {
  "--normal-bg": "var(--card)",
  "--normal-text": "var(--card-foreground)",
  "--normal-border": "var(--border)",
  "--border-radius": "var(--radius-lg)",
  "--gray2": "var(--accent)",
  "--gray5": "var(--border)",
} as CSSProperties;

const CLASS_NAMES = {
  toast: "shadow-lift! font-sans! text-sm! gap-3!",
  title: "text-sm! leading-snug! font-semibold!",
  description: "text-muted-foreground! text-[0.8125rem]!",
  closeButton: "text-muted-foreground! hover:text-foreground!",
  success: "[&_[data-icon]]:text-success",
  error: "[&_[data-icon]]:text-destructive",
  warning: "[&_[data-icon]]:text-warning",
} as const;

const ICONS = {
  success: <CircleCheckIcon aria-hidden className="size-4" />,
  info: <InfoIcon aria-hidden className="size-4" />,
  warning: <TriangleAlertIcon aria-hidden className="size-4" />,
  error: <OctagonXIcon aria-hidden className="size-4" />,
  loading: <Loader2Icon aria-hidden className="size-4 animate-spin" />,
};

/** Bottom right on desktop, top centre on phones; mounted once in the root layout. */
export function Toaster(props: ToasterProps) {
  const t = useTranslations("common");
  const { resolvedTheme } = useTheme();
  const belowMd = useIsBelowMd();
  return (
    <Sonner
      theme={resolvedTheme === "dark" ? "dark" : "light"}
      position={belowMd ? "top-center" : "bottom-right"}
      visibleToasts={MAX_VISIBLE_TOASTS}
      closeButton
      containerAriaLabel={t("notifications")}
      className="toaster pointer-events-auto"
      icons={ICONS}
      style={TOAST_VARS}
      toastOptions={{ classNames: CLASS_NAMES, closeButtonAriaLabel: t("closeNotification") }}
      {...props}
    />
  );
}
