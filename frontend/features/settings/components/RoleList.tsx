"use client";

import { useTranslations } from "next-intl";

import { InlineAlert } from "@/components/ui/InlineAlert";
import type { Model, Provider, RoleAssignment } from "@/features/registry/types";
import type { Loaded } from "@/lib/api/loaded";
import type { OrgModelRole } from "@/lib/core/types";

import type { SettingsScope } from "../contract";
import { SETTINGS_ROLES } from "../role-fit";
import { orgCurrent, platformCurrent } from "../scope-roles";
import type { RoleDraft, RoleChange } from "../useRoleDraft";
import { RoleRow } from "./RoleRow";

type Props = {
  scope: SettingsScope;
  providers: Provider[];
  models: Loaded<Model[]>;
  platformRoles: Loaded<RoleAssignment[]>;
  /** Only read on an org tab. */
  orgRoles: Loaded<OrgModelRole[]> | null;
  draft: RoleDraft;
  onChange: (role: (typeof SETTINGS_ROLES)[number], next: RoleChange | undefined) => void;
  onEditCapabilities?: (model: Model) => void;
  onSyncProvider?: (providerId: string) => Promise<void>;
};

/** A failed load shows its reason in place; never an empty list that would read as "nothing configured". */
function LoadFailed({ title, loaded }: { title: string; loaded: Extract<Loaded<unknown>, { ok: false }> }) {
  return (
    <InlineAlert title={title} detail={loaded.message}>
      {loaded.requestId && <p className="tabular text-muted-foreground mt-0.5 text-xs">{loaded.requestId}</p>}
    </InlineAlert>
  );
}

/** The four role rows for one scope (O6), in display order. */
export function RoleList({
  scope,
  providers,
  models,
  platformRoles,
  orgRoles,
  draft,
  onChange,
  ...rest
}: Props) {
  const t = useTranslations("settings");
  if (!models.ok) return <LoadFailed title={t("rolesFailed")} loaded={models} />;
  if (!platformRoles.ok) return <LoadFailed title={t("rolesFailed")} loaded={platformRoles} />;
  if (scope.scope === "org" && orgRoles && !orgRoles.ok) {
    return <LoadFailed title={t("orgRolesFailed")} loaded={orgRoles} />;
  }
  const fastId = platformCurrent(platformRoles.data, "chat.fast").modelId;
  const fastModel = models.data.find((m) => m.id === fastId);
  return (
    <>
      {SETTINGS_ROLES.map((role, index) => {
        const platform = platformCurrent(platformRoles.data, role);
        const current = scope.scope === "org" && orgRoles?.ok ? orgCurrent(orgRoles.data, role) : platform;
        return (
          <RoleRow
            key={role}
            role={role}
            scope={scope}
            index={index}
            providers={providers}
            models={models.data}
            current={current}
            change={draft[role]}
            onChange={(next) => onChange(role, next)}
            platformModelId={platform.modelId}
            fastModel={fastModel}
            {...rest}
          />
        );
      })}
    </>
  );
}
