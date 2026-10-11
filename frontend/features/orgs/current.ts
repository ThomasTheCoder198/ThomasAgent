import type { Organization } from "./contract";

const ORG_PATH = /^\/orgs\/([^/]+)(?:\/|$)/;

export function orgIdFromPath(path: string): string | null {
  const match = ORG_PATH.exec(path);
  if (!match?.[1]) return null;
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return null; // a malformed escape is just not an org id; proxy.ts must not 500 on it
  }
}

/** Only ids that are in the operator's list count: a stale cookie or a typed URL falls back to the first org. */
export function resolveCurrentOrg(
  orgs: readonly Organization[],
  pathOrgId: string | null,
  cookieOrgId: string | null | undefined,
): Organization | null {
  return (
    orgs.find((org) => org.id === pathOrgId) ?? orgs.find((org) => org.id === cookieOrgId) ?? orgs[0] ?? null
  );
}
