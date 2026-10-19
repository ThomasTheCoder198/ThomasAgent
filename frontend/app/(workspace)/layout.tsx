import { BarChart3, Bot, LibraryBig, Settings, Waypoints } from "lucide-react";
import { cookies, headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import type { ReactNode } from "react";

import { isToolLine } from "@/components/lines";
import { requireUser } from "@/features/auth/session";
import { listRecents } from "@/features/chat/server";
import { orgHome } from "@/features/orgs/contract";
import { OrgPlate } from "@/features/orgs/OrgPlate";
import { getCurrentOrg } from "@/features/orgs/server";
import { SETTINGS_ROUTE } from "@/features/settings/contract";
import { CollapseButton } from "@/features/shell/CollapseButton";
import { ShellFrame } from "@/features/shell/ShellFrame";
import { Sidebar, type NavItem } from "@/features/shell/Sidebar";
import { isCollapsedValue, SIDEBAR_COOKIE } from "@/features/shell/sidebar-state";
import { UserMenu } from "@/features/shell/UserMenu";
import { pathFromHeaders } from "@/lib/request-path";

export default async function WorkspaceLayout({ children }: { children: ReactNode }) {
  const path = pathFromHeaders(await headers());
  const user = await requireUser(path);
  const [t, jar, currentOrg] = await Promise.all([getTranslations(), cookies(), getCurrentOrg()]);
  const { org: current, orgs, failed } = currentOrg;
  const recents = current
    ? (await listRecents(current.id)).map((c) => ({ ...c, lines: c.lines.filter(isToolLine) }))
    : [];
  const items: NavItem[] = [
    { href: "/agents", label: t("shell.agents"), icon: Bot },
    {
      // Straight to the current org's list; /knowledge-bases is the fallback when there is no org yet.
      href: current ? orgHome(current.id) : "/knowledge-bases",
      label: t("shell.knowledgeBases"),
      icon: LibraryBig,
      lines: ["kb"],
      orgScope: { orgIds: orgs.map((org) => org.id), serverOrgId: current?.id ?? null },
    },
    { href: "/tools", label: t("shell.tools"), icon: Waypoints, lines: ["app", "mcp", "cmp"] },
    { href: "/usage", label: t("shell.usage"), icon: BarChart3 },
    { href: SETTINGS_ROUTE, label: t("shell.settings"), icon: Settings },
  ];
  return (
    <ShellFrame
      initialCollapsed={isCollapsedValue(jar.get(SIDEBAR_COOKIE)?.value)}
      labels={{
        openMenu: t("shell.openMenu"),
        closeMenu: t("shell.closeMenu"),
        collapse: t("shell.collapse"),
        expand: t("shell.expand"),
        drawerTitle: t("shell.nav"),
      }}
      sidebar={
        <Sidebar
          items={items}
          recents={recents}
          user={user}
          activeHref={path}
          footer={<UserMenu />}
          org={<OrgPlate orgs={orgs} current={current} failed={failed} />}
          toggle={<CollapseButton />}
          labels={{
            newChat: t("shell.newChat"),
            search: t("shell.search"),
            searchSoon: t("shell.searchSoon"),
            recents: t("shell.recents"),
            recentsEmpty: t("empty.recentsEmpty"),
            recentsRetention: t("shell.recentsRetention"),
            tenant: t("shell.tenant"),
            nav: t("shell.nav"),
          }}
        />
      }
    >
      {children}
    </ShellFrame>
  );
}
