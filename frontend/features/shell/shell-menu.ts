"use client";

import { createContext, useContext } from "react";

export type ShellMenu = {
  /** Mobile drawer. */
  open: () => void;
  label: string;
  expanded: boolean;
  /** Desktop rail. */
  collapsed: boolean;
  toggleCollapsed: () => void;
  collapseLabel: string;
  expandLabel: string;
};

/** Lets a page header carry the mobile menu button and the sidebar its collapse toggle. */
export const ShellMenuContext = createContext<ShellMenu | null>(null);

export function useShellMenu() {
  return useContext(ShellMenuContext);
}

/** Routes whose page header owns the mobile bar (menu button + title), so phones get one bar, not two. */
const OWN_HEADER_PREFIXES = ["/chat", "/agents"] as const;

export function pageOwnsHeader(pathname: string): boolean {
  return OWN_HEADER_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}
