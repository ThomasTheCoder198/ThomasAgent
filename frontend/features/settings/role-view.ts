import type { SettingsScope } from "./contract";
import type { RoleChange } from "./useRoleDraft";

/** What the role resolves to now in this scope; `source` is where the model comes from (core's OrgModelRole). */
export type RoleCurrent = { modelId: string | null; source: "org" | "platform" | "fallback" | "none" };

/** The scope's own assignment: on an org tab only an `org` source is the org's; anything else is inherited. */
export function ownModelId(scope: SettingsScope, current: RoleCurrent): string | null {
  if (scope.scope === "org") return current.source === "org" ? current.modelId : null;
  return current.modelId;
}

/** The model id after the draft's edit, if any; null means "no own model". */
export function chosenModelId(scope: SettingsScope, current: RoleCurrent, change: RoleChange | undefined) {
  if (!change) return ownModelId(scope, current);
  return change.kind === "set" ? change.modelId : null;
}

/** The model an org falls back to while it has no override of its own. */
export function inheritedModelId(current: RoleCurrent, platformModelId: string | null | undefined) {
  if (current.source === "platform" || current.source === "fallback") return current.modelId;
  return platformModelId ?? null;
}

/**
 * The change that makes the scope's stored own model go away: nothing to send when there is none
 * (an unassigned platform role, or an org that already inherits), otherwise a clear.
 */
export function dropChange(scope: SettingsScope, current: RoleCurrent): RoleChange | undefined {
  return ownModelId(scope, current) ? { kind: "clear" } : undefined;
}

export type RoleViewKind = "missing" | "inherited" | "fast" | "edit";

/**
 * Which of the row's four faces to draw. `chosenId` with no model behind it is a deleted model; an org with
 * no model of its own follows the platform until it starts overriding; the platform decision role can follow
 * the fast chat model; otherwise the provider / model / Test controls.
 */
export function roleView(input: {
  scope: SettingsScope;
  role: string;
  chosenId: string | null;
  modelExists: boolean;
  overriding: boolean;
  ownMode: boolean;
}): RoleViewKind {
  const { scope, chosenId } = input;
  if (chosenId && !input.modelExists) return "missing";
  if (scope.scope === "org") return chosenId || input.overriding ? "edit" : "inherited";
  if (input.role === "decision" && !chosenId && !input.ownMode) return "fast";
  return "edit";
}

/** The provider the row starts on: the chosen model's, else the one an org is about to override. */
export function initialProviderId(
  models: { id: string; providerId: string }[],
  chosenId: string | null,
  inheritedId: string | null,
): string | undefined {
  const byId = (id: string | null) => models.find((m) => m.id === id)?.providerId;
  return byId(chosenId) ?? byId(inheritedId);
}
