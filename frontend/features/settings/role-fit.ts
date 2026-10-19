import type { Capability, Model, Role } from "@/features/registry/types";

/** The core roles the Settings page sets, in display order (O6). Embedding and rerank belong to ragx. */
export const SETTINGS_ROLES = [
  "chat.default",
  "chat.fast",
  "decision",
  "vision",
] as const satisfies readonly Role[];
export type SettingsRole = (typeof SETTINGS_ROLES)[number];

/**
 * Mirrors core `model.RequirementsForRole` (backend-core/internal/llmprovider/model/role.go): every listed
 * capability is needed. Core re-checks on save (LLM_MODEL_CAPABILITY_MISMATCH), so this only orders the list.
 */
const REQUIRED: Record<SettingsRole, readonly Capability[]> = {
  "chat.default": ["chat", "tools"],
  "chat.fast": ["chat"],
  decision: ["decision"],
  vision: ["vision"],
};

export function fitsRole(model: Model, role: SettingsRole): boolean {
  return REQUIRED[role].every((c) => model.capabilities.includes(c));
}

/** What a model lacks for a role, for the "not capable" rows of the picker. */
export function missingCapabilities(model: Model, role: SettingsRole): Capability[] {
  return REQUIRED[role].filter((c) => !model.capabilities.includes(c));
}
