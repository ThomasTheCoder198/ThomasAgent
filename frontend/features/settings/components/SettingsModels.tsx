"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Tabs, TabsContent } from "@/components/ui/Tabs";
import { SaveBar } from "@/features/agents/components/SaveBar";
import type { Model } from "@/features/registry/types";
import { PageHeader } from "@/features/shell/PageHeader";
import { loadedOr } from "@/lib/api/loaded";

import { orgOverrideCount } from "../scope-roles";
import type { SettingsData } from "../server";
import { useSettingsPage, type SettingsTab } from "../useSettingsPage";
import { CapabilityDialog } from "./CapabilityDialog";
import { ProviderDialog } from "./ProviderDialog";
import { ProvidersSection } from "./ProvidersSection";
import { RoleList } from "./RoleList";
import { DiscardDialog, NoOrg } from "./ScopeParts";
import { ScopeHints, ScopeTabs } from "./ScopeTabs";

type Props = SettingsData & {
  /** The org picked in the sidebar switcher; null when core lists none. */
  org: { id: string; name: string } | null;
  /** The org list itself failed to load, which is different from "no org yet". */
  orgsFailed: boolean;
};

const NO_ERRORS = {};

/**
 * One page, two scope tabs (owner 2026-10-18): the platform default and the org chosen in the sidebar switcher.
 * The draft belongs to the tab being shown; leaving a dirty tab asks first. Providers sit below on every tab.
 */
export function SettingsModels({ org, orgsFailed, providers, models, platformRoles, orgRoles }: Props) {
  const t = useTranslations("settings");
  const page = useSettingsPage(org?.id ?? null, { platformRoles, orgRoles });
  const [addingProvider, setAddingProvider] = useState(false);
  const [editingModel, setEditingModel] = useState<Model | null>(null);
  const overrides = orgOverrideCount(loadedOr(orgRoles ?? { ok: true, data: [] }, []));

  return (
    <div className="h-full overflow-y-auto">
      <Tabs
        value={page.tab}
        onValueChange={(next) => page.changeTab(next as SettingsTab)}
        className="min-h-full gap-0"
      >
        <PageHeader
          title={t("title")}
          body={t("subtitle")}
          actions={
            <Button variant="outline" onClick={() => setAddingProvider(true)}>
              <Plus aria-hidden />
              {t("addProvider")}
            </Button>
          }
        />
        <ScopeTabs orgName={org?.name} overrides={overrides} />
        <TabsContent
          key={`${page.tab}-${page.resetKey}`}
          value={page.tab}
          className="mx-auto w-full max-w-5xl flex-none px-4 pt-6 outline-none md:px-6 lg:px-8"
        >
          <ScopeHints tab={page.tab} />
          {page.editable ? (
            <RoleList
              scope={page.scope}
              providers={loadedOr(providers, [])}
              models={models}
              platformRoles={platformRoles}
              orgRoles={orgRoles}
              draft={page.draft.draft}
              onChange={page.draft.change}
              onEditCapabilities={setEditingModel}
              onSyncProvider={page.syncOne}
            />
          ) : (
            <NoOrg failed={orgsFailed} />
          )}
        </TabsContent>
        <ProvidersSection providers={providers} models={models} />
        <SaveBar
          dirty={page.draft.dirty}
          canSave={page.editable}
          errors={NO_ERRORS}
          saving={page.saving}
          onSave={() => void page.save()}
          onDiscard={page.discard}
        />
      </Tabs>
      {addingProvider && <ProviderDialog onOpenChange={setAddingProvider} />}
      {editingModel && (
        <CapabilityDialog model={editingModel} onOpenChange={(open) => !open && setEditingModel(null)} />
      )}
      {page.pendingTab && <DiscardDialog onCancel={page.cancelTabChange} onConfirm={page.confirmTabChange} />}
    </div>
  );
}
