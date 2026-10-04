"use client";

import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";

import { InlineAlert } from "@/components/ui/InlineAlert";
import { SettingsSection } from "@/components/ui/SettingsSection";
import type { Loaded } from "@/lib/api/loaded";
import type { Capability, Model, RoleAssignment } from "@/features/registry/types";

import { agentRoles, type AgentRole } from "../contract";
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
/** The select's value for "no override, inherit the platform role". */
const INHERIT = "";

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
  function setOverride(role: AgentRole, modelId: string) {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars -- dropped on purpose: the role inherits again
    const { [role]: _removed, ...rest } = draft.overrides;
    update({ overrides: modelId === INHERIT ? rest : { ...rest, [role]: modelId } });
  }
  return (
    <>
      <p className="text-ink-2 max-w-[70ch] pt-2 pb-2 text-[13.5px]">{t("modelsHint")}</p>
      {agentRoles.map((role) => {
        const keys = roleKeys[role];
        const platformModel = models.find((m) => m.id === roles.find((r) => r.role === role)?.modelId);
        const choices = models.filter((m) => m.capabilities.includes(roleCapability[role]));
        const selectId = `role-${role}`;
        return (
          <SettingsSection key={role} title={t(keys.label)} hint={t(keys.hint)}>
            <label htmlFor={selectId} className="sr-only">
              {t(keys.label)}
            </label>
            <div className="relative max-w-md">
              <select
                id={selectId}
                value={draft.overrides[role] ?? INHERIT}
                onChange={(e) => setOverride(role, e.target.value)}
                className="border-rule-strong bg-paper text-ink focus-visible:border-ink w-full cursor-pointer appearance-none rounded-lg border py-2.5 pr-9 pl-3 text-[15px] outline-none"
              >
                <option value={INHERIT}>
                  {platformModel ? t("inherit", { model: platformModel.displayName }) : t("inheritNone")}
                </option>
                {choices.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.displayName} · {m.modelRef}
                  </option>
                ))}
              </select>
              <ChevronDown
                aria-hidden
                className="text-ink-3 pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2"
              />
            </div>
          </SettingsSection>
        );
      })}
    </>
  );
}
