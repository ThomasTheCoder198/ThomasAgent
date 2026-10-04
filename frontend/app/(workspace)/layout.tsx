import { BarChart3, Bot, LibraryBig, Waypoints } from "lucide-react";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import type { ReactNode } from "react";

import { requireUser } from "@/features/auth/session";
import { listRecents } from "@/features/chat/server";
import { ShellFrame } from "@/features/shell/ShellFrame";
import { Sidebar, type NavItem } from "@/features/shell/Sidebar";
import { UserMenu } from "@/features/shell/UserMenu";
import { pathFromHeaders } from "@/lib/request-path";

export default async function WorkspaceLayout({ children }: { children: ReactNode }) {
  const path = pathFromHeaders(await headers());
  const user = await requireUser(path);
  const [t, recents] = await Promise.all([getTranslations(), listRecents()]);
  const items: NavItem[] = [
    { href: "/agents", label: t("shell.agents"), icon: Bot },
    { href: "/knowledge-bases", label: t("shell.knowledgeBases"), icon: LibraryBig, lines: ["kb"] },
    { href: "/tools", label: t("shell.tools"), icon: Waypoints, lines: ["app", "mcp", "cmp"] },
    { href: "/usage", label: t("shell.usage"), icon: BarChart3 },
  ];
  return (
    <ShellFrame
      labels={{ openMenu: t("shell.openMenu"), closeMenu: t("shell.closeMenu") }}
      sidebar={
        <Sidebar
          items={items}
          recents={recents}
          user={user}
          activeHref={path}
          footer={<UserMenu />}
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
