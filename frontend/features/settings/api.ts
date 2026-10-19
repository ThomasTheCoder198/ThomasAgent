import {
  PROVIDER_PATHS,
  type Capability,
  type Provider,
  type ProviderInput,
} from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";
import type { ModelTestResult } from "@/lib/core/types";

import type { SaveRequest } from "./useRoleDraft";

/** Saves role changes one by one; the first failure stops and is reported, earlier ones stay saved. */
export async function saveRoles(requests: SaveRequest[], locale: Locale): Promise<void> {
  for (const request of requests) {
    await apiFetch<unknown>(request.path, {
      method: request.method,
      body: request.method === "PUT" ? JSON.stringify(request.body) : undefined,
      locale,
    });
  }
}

export const testModel = (id: string, locale: Locale) =>
  apiFetch<ModelTestResult>(PROVIDER_PATHS.modelTest(id), { method: "POST", locale });

export const createProvider = (input: ProviderInput, locale: Locale) =>
  apiFetch<Provider>(PROVIDER_PATHS.providers, { method: "POST", body: JSON.stringify(input), locale });

export const updateProvider = (id: string, input: ProviderInput, locale: Locale) =>
  apiFetch<Provider>(PROVIDER_PATHS.provider(id), { method: "PATCH", body: JSON.stringify(input), locale });

export const deleteProvider = (id: string, locale: Locale) =>
  apiFetch<unknown>(PROVIDER_PATHS.provider(id), { method: "DELETE", locale });

export const syncProvider = (id: string, locale: Locale) =>
  apiFetch<{ added: number; updated: number; skipped: number }>(PROVIDER_PATHS.sync(id), {
    method: "POST",
    locale,
  });

/** Q12: O8's "Sửa" replaces the model's capabilities; core refuses a change that would break a role. */
export const updateCapabilities = (id: string, capabilities: Capability[], locale: Locale) =>
  apiFetch<unknown>(PROVIDER_PATHS.model(id), {
    method: "PATCH",
    body: JSON.stringify({ capabilities }),
    locale,
  });
