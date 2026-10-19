import { PROVIDER_PATHS, type ProviderKind, type Role } from "@/features/registry/types";

/** Core's model-role paths (index R1, ruling C2-11). Providers and models stay platform-wide. */
const enc = encodeURIComponent;

export const ORG_ROLE_PATHS = {
  roles: (orgId: string) => `/api/v1/orgs/${enc(orgId)}/model-roles`,
  role: (orgId: string, role: string) => `/api/v1/orgs/${enc(orgId)}/model-roles/${enc(role)}`,
} as const;

/** Owner 2026-10-18: the page lives at `/settings` (the plan's `/settings/models` is replaced). */
export const SETTINGS_ROUTE = "/settings";

export type SettingsScope = { scope: "platform" } | { scope: "org"; orgId: string };

export const rolePath = (scope: SettingsScope, role: Role) =>
  scope.scope === "platform" ? PROVIDER_PATHS.role(role) : ORG_ROLE_PATHS.role(scope.orgId, role);

/**
 * Provider form rules, mirrored from core (`provider_service.go`: kinds without a default base URL need one).
 * The key is optional for self-hosted kinds. Core stays the authority.
 */
export const KINDS_NEEDING_BASE_URL: ReadonlySet<ProviderKind> = new Set([
  "azure",
  "ollama",
  "openai_compatible",
]);
export const KINDS_WITHOUT_KEY: ReadonlySet<ProviderKind> = new Set(["ollama"]);
export const KINDS_WITH_OPTIONAL_KEY: ReadonlySet<ProviderKind> = new Set(["openai_compatible"]);
/** Kinds whose models answer core roles; embedding-only vendors (cohere, voyage, jina) are configured in ragx. */
export const CHAT_PROVIDER_KINDS: readonly ProviderKind[] = [
  "openrouter",
  "openai",
  "anthropic",
  "google",
  "azure",
  "ollama",
  "openai_compatible",
];
