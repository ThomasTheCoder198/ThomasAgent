"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

export type MatchMode = "exact" | "section";

export function isActivePath(href: string, path: string, match: MatchMode) {
  return path === href || (match === "section" && path.startsWith(`${href}/`));
}

type Props = { href: string; match: MatchMode; serverPath: string; className?: string; children: ReactNode };

/**
 * Layouts persist across client navigations, so the server-rendered path goes stale; the live pathname
 * (which also follows `history.replaceState` when a new chat gets its id) decides `aria-current`.
 */
export function NavLink({ href, match, serverPath, className, children }: Props) {
  const path = usePathname() ?? serverPath;
  return (
    <Link
      href={href}
      aria-current={isActivePath(href, path, match) ? "page" : undefined}
      className={className}
    >
      {children}
    </Link>
  );
}
