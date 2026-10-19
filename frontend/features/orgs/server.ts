import "server-only";

import { cookies, headers } from "next/headers";

import { serverFetchOptional, serverFetchResult, type Loaded } from "@/lib/api/server";
import { pathFromHeaders } from "@/lib/request-path";

import { CURRENT_ORG_COOKIE, ORG_PATHS, type Organization } from "./contract";
import { orgIdFromPath, resolveCurrentOrg } from "./current";

export function listOrganizations(): Promise<Loaded<Organization[]>> {
  return serverFetchResult<Organization[]>(ORG_PATHS.orgs);
}

/** null means core answered 404: the org does not exist (or is not an org id). Other failures throw. */
export function getOrganization(id: string): Promise<Organization | null> {
  return serverFetchOptional<Organization | null>(ORG_PATHS.org(id), null);
}

/** `failed` is a 5xx or an unreachable core (never "no org"); `requestId` is shown with the retry. */
export type CurrentOrg = {
  org: Organization | null;
  orgs: Organization[];
  failed: boolean;
  requestId?: string;
};

/**
 * The org the business pages (/chat, /agents, /settings) talk to (ruling C2-1): the one in the path, else the
 * last one viewed (cookie), else the first. Core re-checks the org on every request, so this only picks a default.
 */
export async function getCurrentOrg(): Promise<CurrentOrg> {
  const [loaded, jar, requestHeaders] = await Promise.all([listOrganizations(), cookies(), headers()]);
  if (!loaded.ok) return { org: null, orgs: [], failed: true, requestId: loaded.requestId };
  const path = pathFromHeaders(requestHeaders);
  const org = resolveCurrentOrg(loaded.data, orgIdFromPath(path), jar.get(CURRENT_ORG_COOKIE)?.value);
  return { org, orgs: loaded.data, failed: false };
}
