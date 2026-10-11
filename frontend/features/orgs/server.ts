import "server-only";

import { serverFetchOptional, serverFetchResult, type Loaded } from "@/lib/api/server";

import { ORG_PATHS, type Organization } from "./contract";

export function listOrganizations(): Promise<Loaded<Organization[]>> {
  return serverFetchResult<Organization[]>(ORG_PATHS.orgs);
}

/** null means core answered 404: the org does not exist (or is not an org id). Other failures throw. */
export function getOrganization(id: string): Promise<Organization | null> {
  return serverFetchOptional<Organization | null>(ORG_PATHS.org(id), null);
}
