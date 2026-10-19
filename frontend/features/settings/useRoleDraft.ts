"use client";

import { useCallback, useState } from "react";

import { rolePath, type SettingsScope } from "./contract";
import { SETTINGS_ROLES, type SettingsRole } from "./role-fit";

export type RoleChange = { kind: "set"; modelId: string } | { kind: "clear" };
export type RoleDraft = Partial<Record<SettingsRole, RoleChange>>;
export type SaveRequest =
  { method: "PUT"; path: string; body: { modelId: string } } | { method: "DELETE"; path: string };

/** The requests that make the stored roles equal the draft, in role display order. */
export function planSave(scope: SettingsScope, draft: RoleDraft): SaveRequest[] {
  return SETTINGS_ROLES.flatMap((role): SaveRequest[] => {
    const change = draft[role];
    if (!change) return [];
    const path = rolePath(scope, role);
    return change.kind === "set"
      ? [{ method: "PUT", path, body: { modelId: change.modelId } }]
      : [{ method: "DELETE", path }];
  });
}

/** Local edits for one scope tab; switching tabs with unsaved edits asks first (the page owns that). */
export function useRoleDraft() {
  const [draft, setDraft] = useState<RoleDraft>({});
  return {
    draft,
    dirty: Object.keys(draft).length > 0,
    change: (role: SettingsRole, next: RoleChange | undefined) =>
      setDraft((d) => {
        const rest = Object.fromEntries(Object.entries(d).filter(([key]) => key !== role)) as RoleDraft;
        return next ? { ...rest, [role]: next } : rest;
      }),
    reset: useCallback(() => setDraft({}), []),
  };
}
