import "server-only";

import {
  PROVIDER_PATHS,
  REGISTRY_PATHS,
  type Model,
  type Provider,
  type RoleAssignment,
} from "@/features/registry/types";
import { serverFetchResult, type Loaded } from "@/lib/api/server";
import type { OrgModelRole } from "@/lib/core/types";

import { ORG_ROLE_PATHS } from "./contract";

export type SettingsData = {
  providers: Loaded<Provider[]>;
  models: Loaded<Model[]>;
  platformRoles: Loaded<RoleAssignment[]>;
  /** The current org's roles, loaded with the rest so its tab can show an override count; null without an org. */
  orgRoles: Loaded<OrgModelRole[]> | null;
};

export async function getSettingsData(orgId: string | null): Promise<SettingsData> {
  const [providers, models, platformRoles, orgRoles] = await Promise.all([
    serverFetchResult<Provider[]>(PROVIDER_PATHS.providers),
    serverFetchResult<Model[]>(REGISTRY_PATHS.models),
    serverFetchResult<RoleAssignment[]>(REGISTRY_PATHS.roles),
    orgId ? serverFetchResult<OrgModelRole[]>(ORG_ROLE_PATHS.roles(orgId)) : Promise.resolve(null),
  ]);
  return { providers, models, platformRoles, orgRoles };
}
