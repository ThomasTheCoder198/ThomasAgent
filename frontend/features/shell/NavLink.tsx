"use client";

import { motion } from "motion/react";
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
  /** Draws the 3px violet bar that slides to whichever row is current; rows in one `NavGroup` share it. */
  indicator?: boolean;
  children: ReactNode;
};

/** The link target: `href`, or the org home of the org the URL currently names when the link is org-scoped. */
function useOrgTarget(href: string, orgScope: OrgScope | undefined) {
  const liveOrgId = useLiveOrgId(orgScope?.orgIds ?? [], orgScope?.serverOrgId ?? null);
  return orgScope && liveOrgId ? orgHome(liveOrgId) : href;
}

/** The 3px violet bar; the one in the current row slides to wherever the active row moves. */
function NavIndicator() {
  return (
    <motion.span
      aria-hidden
      layoutId="nav-indicator"
      className="bg-primary absolute inset-y-2 left-0 w-[3px] rounded-full"
    />
  );
}

/**
 * Layouts persist across client navigations, so the server-rendered path goes stale; the live pathname
 * (which also follows `history.replaceState` when a new chat gets its id) decides `aria-current`.
 */
export function NavLink({ href, orgScope, match, serverPath, title, className, indicator, children }: Props) {
  const path = usePathname() ?? serverPath;
  const target = useOrgTarget(href, orgScope);
  const active = isActivePath(target, path, match);
  return (
    <Link href={target} title={title} aria-current={active ? "page" : undefined} className={className}>
      {indicator && active && <NavIndicator />}
      {children}
    </Link>
  );
}
