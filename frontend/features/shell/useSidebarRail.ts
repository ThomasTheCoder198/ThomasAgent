"use client";

import { useCallback, useEffect, useState } from "react";

import { isEditableTarget, isToggleShortcut, writeSidebarCookie } from "./sidebar-state";

/** The desktop rail preference: persisted in a cookie and toggled with Cmd/Ctrl+B. */
export function useSidebarRail(initialCollapsed: boolean) {
  const [collapsed, setCollapsed] = useState(initialCollapsed);

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

  return { collapsed, toggleCollapsed };
}
