"use client";

import { usePathname } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { useDismiss } from "@/components/ui/useDismiss";

import type { ShellMenu } from "./shell-menu";
import { isEditableTarget, isToggleShortcut, writeSidebarCookie } from "./sidebar-state";

type Labels = { openMenu: string; collapse: string; expand: string };

/** Mobile drawer open/close plus the desktop rail preference (persisted, ⌘/Ctrl+B). */
export function useShellState(initialCollapsed: boolean, labels: Labels) {
  const [open, setOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const pathname = usePathname() ?? "";
  const [shownPath, setShownPath] = useState(pathname);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, close);
  // Navigating from inside the drawer should land on the page, not leave the drawer covering it.
  if (pathname !== shownPath) {
    setShownPath(pathname);
    setOpen(false);
  }

  // Side effects stay outside the state updater: StrictMode may run updaters twice.
  const toggleCollapsed = useCallback(() => {
    const next = !collapsed;
    setCollapsed(next);
    writeSidebarCookie(next);
  }, [collapsed]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (!isToggleShortcut(event) || isEditableTarget(event.target)) return;
      event.preventDefault();
      toggleCollapsed();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggleCollapsed]);

  const menu = useMemo<ShellMenu>(
    () => ({
      open: () => setOpen(true),
      label: labels.openMenu,
      expanded: open,
      collapsed,
      toggleCollapsed,
      collapseLabel: labels.collapse,
      expandLabel: labels.expand,
    }),
    [labels.openMenu, labels.collapse, labels.expand, open, collapsed, toggleCollapsed],
  );

  return { open, close, collapsed, pathname, menu };
}
