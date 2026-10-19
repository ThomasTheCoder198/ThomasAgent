import type { RoleAssignment } from "@/features/registry/types";
import type { OrgModelRole } from "@/lib/core/types";

import type { SettingsRole } from "./role-fit";
import type { RoleCurrent } from "./role-view";

/** The platform tab's view of a role: assigned or not. */
export function platformCurrent(roles: RoleAssignment[], role: SettingsRole): RoleCurrent {
  const modelId = roles.find((r) => r.role === role)?.modelId ?? null;
  return { modelId, source: modelId ? "platform" : "none" };
}

/** The org tab's view: the org's own model when it overrides, otherwise what a run would use now. */
export function orgCurrent(roles: OrgModelRole[], role: SettingsRole): RoleCurrent {
  const found = roles.find((r) => r.role === role);
  if (!found) return { modelId: null, source: "none" };
  return { modelId: found.source === "org" ? found.modelId : found.effectiveModelId, source: found.source };
}

/** How many roles an org overrides; the tab shows it so the operator sees where the platform default is not used. */
export function orgOverrideCount(roles: OrgModelRole[] | undefined): number {
  return roles?.filter((r) => r.source === "org").length ?? 0;
}
