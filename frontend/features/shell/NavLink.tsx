"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { orgHome } from "@/features/orgs/contract";
import { useLiveOrgId } from "@/features/orgs/useLiveOrgId";

export type MatchMode = "exact" | "section";

export function isActivePath(href: string, path: string, match: MatchMode) {
  return path === href || (match === "section" && path.startsWith(`${href}/`));
}

/** Data for a link into the current org: a server component cannot pass a function, so the ids travel instead. */
export type OrgScope = { orgIds: readonly string[]; serverOrgId: string | null };

type Props = {
  href: string;
  orgScope?: OrgScope;
  match: MatchMode;
  serverPath: string;
  title?: string;
  className?: string;
  children: ReactNode;
};

/**
 * Layouts persist across client navigations, so the server-rendered path goes stale; the live pathname
 * (which also follows `history.replaceState` when a new chat gets its id) decides `aria-current`.
 */
export function NavLink({ href, orgScope, match, serverPath, title, className, children }: Props) {
  const path = usePathname() ?? serverPath;
  const liveOrgId = useLiveOrgId(orgScope?.orgIds ?? [], orgScope?.serverOrgId ?? null);
  const target = orgScope && liveOrgId ? orgHome(liveOrgId) : href;
  return (
    <Link
      href={target}
      title={title}
      aria-current={isActivePath(target, path, match) ? "page" : undefined}
      className={className}
    >
      {children}
    </Link>
  );
}
