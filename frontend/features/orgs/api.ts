import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";

import { ORG_PATHS, type Organization, type OrganizationInput } from "./contract";

export function createOrganization(input: OrganizationInput, locale: Locale): Promise<Organization> {
  return apiFetch(ORG_PATHS.orgs, { method: "POST", body: JSON.stringify(input), locale });
}

export function updateOrganization(
  id: string,
  input: Partial<OrganizationInput>,
  locale: Locale,
): Promise<Organization> {
  return apiFetch(ORG_PATHS.org(id), { method: "PATCH", body: JSON.stringify(input), locale });
}

/** Provisions again synchronously; resolves to the org's real state (ready, or failed with ragxErrorCode). */
export function retryOrganization(id: string, locale: Locale): Promise<Organization> {
  return apiFetch(ORG_PATHS.provision(id), { method: "POST", locale });
}
