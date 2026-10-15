import { BarChart3, Bot, LibraryBig, Waypoints } from "lucide-react";
import { cookies, headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import type { ReactNode } from "react";

import { requireUser } from "@/features/auth/session";
import { listRecents } from "@/features/chat/server";
import { CURRENT_ORG_COOKIE, orgHome } from "@/features/orgs/contract";
import { orgIdFromPath, resolveCurrentOrg } from "@/features/orgs/current";
import { OrgPlate } from "@/features/orgs/OrgPlate";
import { listOrganizations } from "@/features/orgs/server";
import { CollapseButton } from "@/features/shell/CollapseButton";
import { ShellFrame } from "@/features/shell/ShellFrame";
import { Sidebar, type NavItem } from "@/features/shell/Sidebar";
import { isCollapsedValue, SIDEBAR_COOKIE } from "@/features/shell/sidebar-state";
import { UserMenu } from "@/features/shell/UserMenu";
import { pathFromHeaders } from "@/lib/request-path";

export default async function WorkspaceLayout({ children }: { children: ReactNode }) {
  const path = pathFromHeaders(await headers());
  const user = await requireUser(path);
  const [t, recents, jar, orgsLoaded] = await Promise.all([
    getTranslations(),
    listRecents(),
    cookies(),
    listOrganizations(),
  ]);
  const orgs = orgsLoaded.ok ? orgsLoaded.data : [];
  const current = resolveCurrentOrg(orgs, orgIdFromPath(path), jar.get(CURRENT_ORG_COOKIE)?.value);
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
          org={<OrgPlate orgs={orgs} current={current} failed={!orgsLoaded.ok} />}
          toggle={<CollapseButton />}
          labels={{
            newChat: t("shell.newChat"),
            search: t("shell.search"),
            searchSoon: t("shell.searchSoon"),
            recents: t("shell.recents"),
            recentsEmpty: t("empty.recentsEmpty"),
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
