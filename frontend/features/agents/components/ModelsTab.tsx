"use client";

import { useTranslations } from "next-intl";

import { Field } from "@/components/ui/Field";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import { SettingsSection } from "@/components/ui/SettingsSection";
import type { Loaded } from "@/lib/api/loaded";
import type { Capability, Model, RoleAssignment } from "@/features/registry/types";

import { agentRoles, type AgentRole } from "../contract";
import { INHERIT_VALUE, toSelectValue, withOverride } from "../model-choice";
import type { TabProps } from "./tabs";

const roleCapability: Record<AgentRole, Capability> = {
  "chat.default": "chat",
  "chat.fast": "chat",
  vision: "vision",
};
const roleKeys = {
  "chat.default": { label: "roleChatDefault", hint: "roleChatDefaultHint" },
  "chat.fast": { label: "roleChatFast", hint: "roleChatFastHint" },
  vision: { label: "roleVision", hint: "roleVisionHint" },
} as const;
type Props = TabProps & { models: Loaded<Model[]>; roles: Loaded<RoleAssignment[]> };

export function ModelsTab({ draft, update, models: loadedModels, roles: loadedRoles }: Props) {
  const t = useTranslations("agents");
  if (!loadedModels.ok || !loadedRoles.ok) {
    const detail = !loadedModels.ok
      ? loadedModels.message
      : !loadedRoles.ok
        ? loadedRoles.message
        : undefined;
    return <InlineAlert title={t("modelsLoadFailed")} detail={detail} />;
  }
  const models = loadedModels.data;
  const roles = loadedRoles.data;
  return (
    <>
      <p className="text-muted-foreground max-w-[70ch] text-sm">{t("modelsHint")}</p>
      {agentRoles.map((role) => {
        const keys = roleKeys[role];
        const platformModel = models.find((m) => m.id === roles.find((r) => r.role === role)?.modelId);
        const choices = models.filter((m) => m.capabilities.includes(roleCapability[role]));
        return (
          <SettingsSection key={role} title={t(keys.label)} hint={t(keys.hint)}>
            <Field label={t(keys.label)} hideLabel>
              {(control) => (
                <Select
                  value={toSelectValue(draft.overrides[role])}
                  onValueChange={(selected) =>
                    update({ overrides: withOverride(draft.overrides, role, selected) })
                  }
                >
                  <SelectTrigger {...control} className="w-full max-w-md">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={INHERIT_VALUE}>
                      {platformModel ? t("inherit", { model: platformModel.displayName }) : t("inheritNone")}
                    </SelectItem>
                    {choices.map((m) => (
                      <SelectItem key={m.id} value={m.id}>
                        {m.displayName} · {m.modelRef}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </Field>
          </SettingsSection>
        );
      })}
    </>
  );
}
