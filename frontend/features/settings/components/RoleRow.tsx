"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";

import { SegmentedChoice } from "@/components/ui/SegmentedChoice";
import { SettingsSection } from "@/components/ui/SettingsSection";
import type { Model, Provider } from "@/features/registry/types";

import type { SettingsScope } from "../contract";
import type { SettingsRole } from "../role-fit";
import {
  chosenModelId,
  dropChange,
  inheritedModelId,
  initialProviderId,
  roleView,
  type RoleCurrent,
} from "../role-view";
import type { RoleChange } from "../useRoleDraft";
import { EditableRole, FastNote, InheritedLine, MissingModel } from "./RoleBody";

export type { RoleCurrent } from "../role-view";

type Props = {
  role: SettingsRole;
  scope: SettingsScope;
  providers: Provider[];
  models: Model[];
  /** What the role resolves to now in this scope. */
  current: RoleCurrent;
  /** The unsaved edit for this role, if any. */
  change: RoleChange | undefined;
  onChange: (next: RoleChange | undefined) => void;
  /** The platform's own assignment, for "Overriding the platform default (X)" and for undoing an override. */
  platformModelId?: string | null;
  /** The platform's fast chat model: what an unset decision role uses. */
  fastModel?: Model;
  index?: number;
  onEditCapabilities?: (model: Model) => void;
  onSyncProvider?: (providerId: string) => Promise<void>;
};

const nameOf = (models: Model[], id: string | null | undefined) =>
  models.find((m) => m.id === id)?.displayName;

/**
 * One role (O6): what it is on the left; on the right one of: a missing model, the platform default an org
 * inherits, the platform decision's "use the fast model" note, or the provider / model / Test controls.
 */
export function RoleRow(props: Props) {
  const { role, scope, models, current, change, onChange, platformModelId } = props;
  const t = useTranslations("settings");
  const chosenId = chosenModelId(scope, current, change);
  const chosen = models.find((m) => m.id === chosenId);
  const inheritedId = inheritedModelId(current, platformModelId);
  const [overriding, setOverriding] = useState(false);
  const [ownMode, setOwnMode] = useState(false);
  const [providerId, setProviderId] = useState(() => initialProviderId(models, chosenId, inheritedId));
  const view = roleView({ scope, role, chosenId, modelExists: !!chosen, overriding, ownMode });
  const label = t(`role.${role}`);

  function startOverride() {
    setOverriding(true);
    if (change) onChange(undefined);
    setProviderId((p) => p ?? initialProviderId(models, null, inheritedId));
  }

  const inheritedName = nameOf(models, inheritedId);
  const faces = {
    missing: <MissingModel onClear={() => onChange({ kind: "clear" })} />,
    inherited: (
      <InheritedLine
        text={
          inheritedName
            ? t(current.source === "fallback" ? "inheritsFast" : "inheritsPlatform", { model: inheritedName })
            : t("inheritsNothing")
        }
        onOverride={startOverride}
      />
    ),
    fast: <FastNote model={props.fastModel} />,
    edit: (
      <EditableRole
        role={role}
        roleLabel={label}
        scope={scope}
        providers={props.providers}
        models={models}
        current={current}
        providerId={providerId}
        onProviderChange={setProviderId}
        shown={chosen && chosen.providerId === providerId ? chosen : undefined}
        onChange={onChange}
        onDropOverride={() => {
          setOverriding(false);
          onChange(dropChange(scope, current));
        }}
        platformName={nameOf(models, platformModelId)}
        onEditCapabilities={props.onEditCapabilities}
        onSyncProvider={props.onSyncProvider}
      />
    ),
  };

  return (
    <SettingsSection index={props.index} title={label} hint={t(`roleHint.${role}`)}>
      {role === "decision" && scope.scope === "platform" && (
        <SegmentedChoice
          legend={t("decisionSource", { role: label })}
          hideLegend
          value={view === "fast" ? "fast" : "own"}
          options={[
            { value: "fast", label: t("useFast") },
            { value: "own", label: t("pickOwn") },
          ]}
          onChange={(next) => {
            setOwnMode(next === "own");
            if (next === "fast") onChange(dropChange(scope, current));
          }}
          className="mb-4 max-w-sm"
        />
      )}
      {faces[view]}
    </SettingsSection>
  );
}
