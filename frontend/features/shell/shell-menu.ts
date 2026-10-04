"use client";

import { createContext, useContext } from "react";

export type ShellMenu = { open: () => void; label: string; expanded: boolean };

/** Lets a page header carry the mobile menu button so phones get one bar instead of two. */
export const ShellMenuContext = createContext<ShellMenu | null>(null);

export function useShellMenu() {
  return useContext(ShellMenuContext);
}

/** Routes whose page header owns the mobile bar (menu, title, run id, inspector). */
export const OWN_HEADER_PREFIX = "/chat";
