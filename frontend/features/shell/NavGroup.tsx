"use client";

import { LayoutGroup } from "motion/react";
import { useId, type ReactNode } from "react";

/**
 * Scopes the sliding active indicator to one sidebar: the desktop rail and the phone drawer render the same
 * links, and without a group their indicators would try to travel between the two.
 */
export function NavGroup({ children }: { children: ReactNode }) {
  return <LayoutGroup id={useId()}>{children}</LayoutGroup>;
}
