import { validateDisplayName, type NameProblem } from "@/lib/validation";

/** Core's organizations API (contracts/openapi/core.v1.yaml, tag Organizations). */
export const ragxStatuses = ["pending", "ready", "failed"] as const;
export type RagxStatus = (typeof ragxStatuses)[number];

export type Organization = {
  id: string;
  name: string;
  externalParsing: boolean;
  ragxStatus: RagxStatus;
  ragxErrorCode: string | null;
  settingsSynced: boolean;
  defaultKnowledgeBaseId: string | null;
  createdAt: string;
  updatedAt: string;
};

export type OrganizationInput = { name: string; externalParsing: boolean };

export const ORG_PATHS = {
  orgs: "/api/v1/orgs",
  org: (id: string) => `/api/v1/orgs/${encodeURIComponent(id)}`,
  provision: (id: string) => `/api/v1/orgs/${encodeURIComponent(id)}/provision`,
} as const;

/** UI memory of the last org viewed; core re-checks the org on every request, so this never grants access. */
export const CURRENT_ORG_COOKIE = "thomas_org";
/** Core's limit (organization/service maxNameLength, ragx OrgProvision.name). */
export const ORG_NAME_MAX_LENGTH = 200;

export const orgHome = (id: string) => `/orgs/${encodeURIComponent(id)}/knowledge-bases`;

export type OrgNameProblem = NameProblem;

export const validateOrgName = (raw: string): OrgNameProblem | null =>
  validateDisplayName(raw, ORG_NAME_MAX_LENGTH);
