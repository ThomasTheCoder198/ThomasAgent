"use client";

import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/Badge";
import { TabsList, TabsTrigger } from "@/components/ui/Tabs";

import type { SettingsTab } from "../useSettingsPage";

/** The two scope tabs on a sticky rail; the org tab carries its override count so it shows where the default is not used. */
export function ScopeTabs({ orgName, overrides }: { orgName: string | undefined; overrides: number }) {
  const t = useTranslations("settings");
  return (
    <div className="bg-background sticky top-0 z-10">
      <div className="mx-auto w-full max-w-5xl px-4 md:px-6 lg:px-8">
        <TabsList aria-label={t("scopeLabel")}>
          <TabsTrigger value="platform">{t("tabPlatform")}</TabsTrigger>
          <TabsTrigger value="org">
            {orgName ?? t("tabOrgFallback")}
            {overrides > 0 && <Badge>{t("overrideCount", { count: overrides })}</Badge>}
          </TabsTrigger>
        </TabsList>
      </div>
    </div>
  );
}

/** One line on what the scope means; the org tab adds where to change the org. */
export function ScopeHints({ tab }: { tab: SettingsTab }) {
  const t = useTranslations("settings");
  return (
    <>
      <p className="text-muted-foreground mb-6 max-w-[70ch] text-sm">
        {tab === "platform" ? t("platformScopeHint") : t("orgScopeHint")}
      </p>
      {tab === "org" && (
        <p className="text-muted-foreground -mt-4 mb-6 max-w-[70ch] text-sm">{t("orgSwitchHint")}</p>
      )}
    </>
  );
}
